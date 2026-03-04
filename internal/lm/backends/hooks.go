package backends

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/benjaminabbitt/scm/internal/config"
)

// HookWriter writes hooks to backend-specific configuration files.
type HookWriter interface {
	// WriteHooks writes hooks to the backend's config file.
	// It preserves user-defined hooks and adds/updates SCM-managed hooks.
	WriteHooks(cfg *config.HooksConfig, projectDir string) error

	// HooksPath returns the path to the hooks configuration file.
	HooksPath(projectDir string) string
}

// WriteHooks writes hooks for the specified backend.
// If the backend doesn't support hooks, this is a no-op.
func WriteHooks(backendName string, cfg *config.HooksConfig, projectDir string) error {
	writer := GetHookWriter(backendName)
	if writer == nil {
		return nil // Backend doesn't support hooks
	}
	return writer.WriteHooks(cfg, projectDir)
}

// GetHookWriter returns a HookWriter for the named backend, or nil if not supported.
func GetHookWriter(name string) HookWriter {
	switch name {
	case "claude-code":
		return &ClaudeCodeHookWriter{}
	case "gemini":
		return &GeminiHookWriter{}
	default:
		return nil
	}
}

// computeHookHash computes a hash from the hook's defining fields.
func computeHookHash(h config.Hook) string {
	// Create a stable representation for hashing
	parts := []string{
		h.Command,
		h.Matcher,
		h.Type,
		h.Prompt,
		fmt.Sprintf("%d", h.Timeout),
		fmt.Sprintf("%t", h.Async),
	}
	data := strings.Join(parts, "|")
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:8]) // Use first 8 bytes for brevity
}

// ClaudeCodeHookWriter writes hooks to Claude Code's settings.json format.
type ClaudeCodeHookWriter struct{}

// HooksPath returns the path to Claude Code's settings.json file.
func (w *ClaudeCodeHookWriter) HooksPath(projectDir string) string {
	return filepath.Join(projectDir, ".claude", "settings.json")
}

// claudeCodeSettings represents the structure of .claude/settings.json
type claudeCodeSettings struct {
	Hooks map[string][]claudeCodeHookMatcher `json:"hooks,omitempty"`
	// Preserve other settings
	Other map[string]json.RawMessage `json:"-"`
}

// claudeCodeHookMatcher represents a hook matcher entry in Claude Code format.
type claudeCodeHookMatcher struct {
	Matcher string           `json:"matcher,omitempty"`
	Hooks   []claudeCodeHook `json:"hooks"`
}

// claudeCodeHook represents a single hook in Claude Code format.
type claudeCodeHook struct {
	Type    string `json:"type,omitempty"`
	Command string `json:"command,omitempty"`
	Prompt  string `json:"prompt,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
	Async   bool   `json:"async,omitempty"`
	SCM     string `json:"_scm,omitempty"` // Hash identifying SCM-managed hooks
}

// unifiedEventToClaudeCode maps unified event names to Claude Code event names.
var unifiedEventToClaudeCode = map[string]string{
	"pre_tool":       "PreToolUse",
	"post_tool":      "PostToolUse",
	"session_start":  "SessionStart",
	"session_end":    "SessionEnd",
	"pre_shell":      "PreToolUse", // Maps to PreToolUse with Bash matcher
	"post_file_edit": "PostToolUse", // Maps to PostToolUse with Edit|Write matcher
}

// WriteHooks implements HookWriter for Claude Code.
func (w *ClaudeCodeHookWriter) WriteHooks(cfg *config.HooksConfig, projectDir string) error {
	if cfg == nil {
		return nil
	}

	settingsPath := w.HooksPath(projectDir)

	// Ensure .claude directory exists
	claudeDir := filepath.Dir(settingsPath)
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("failed to create .claude directory: %w", err)
	}

	// Load existing settings
	settings, err := w.loadSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("failed to load existing settings: %w", err)
	}

	// Remove old SCM-managed hooks from settings
	w.removeScmHooks(settings)

	// Add SCM hooks from unified config
	w.addUnifiedHooks(settings, cfg.Unified)

	// Add SCM hooks from backend-specific passthrough
	if backendHooks, ok := cfg.Plugins["claude-code"]; ok {
		w.addBackendHooks(settings, backendHooks)
	}

	// Write settings back
	return w.saveSettings(settingsPath, settings)
}

// loadSettings loads existing settings.json or returns empty settings.
func (w *ClaudeCodeHookWriter) loadSettings(path string) (*claudeCodeSettings, error) {
	settings := &claudeCodeSettings{
		Hooks: make(map[string][]claudeCodeHookMatcher),
		Other: make(map[string]json.RawMessage),
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return settings, nil
		}
		return nil, err
	}

	// First unmarshal to get all fields
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse settings.json: %w", err)
	}

	// Extract hooks separately
	if hooksRaw, ok := raw["hooks"]; ok {
		if err := json.Unmarshal(hooksRaw, &settings.Hooks); err != nil {
			return nil, fmt.Errorf("failed to parse hooks: %w", err)
		}
		delete(raw, "hooks")
	}

	// Preserve other fields
	settings.Other = raw

	return settings, nil
}

// saveSettings writes settings back to settings.json.
func (w *ClaudeCodeHookWriter) saveSettings(path string, settings *claudeCodeSettings) error {
	// Build output map starting with preserved fields
	output := make(map[string]interface{})
	for k, v := range settings.Other {
		var val interface{}
		json.Unmarshal(v, &val)
		output[k] = val
	}

	// Add hooks if non-empty
	if len(settings.Hooks) > 0 {
		output["hooks"] = settings.Hooks
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// removeScmHooks removes all hooks with _scm field from settings.
func (w *ClaudeCodeHookWriter) removeScmHooks(settings *claudeCodeSettings) {
	for eventName, matchers := range settings.Hooks {
		var filteredMatchers []claudeCodeHookMatcher
		for _, matcher := range matchers {
			var filteredHooks []claudeCodeHook
			for _, hook := range matcher.Hooks {
				if hook.SCM == "" {
					filteredHooks = append(filteredHooks, hook)
				}
			}
			if len(filteredHooks) > 0 {
				matcher.Hooks = filteredHooks
				filteredMatchers = append(filteredMatchers, matcher)
			}
		}
		if len(filteredMatchers) > 0 {
			settings.Hooks[eventName] = filteredMatchers
		} else {
			delete(settings.Hooks, eventName)
		}
	}
}

// addUnifiedHooks translates unified hooks to Claude Code format and adds them.
func (w *ClaudeCodeHookWriter) addUnifiedHooks(settings *claudeCodeSettings, unified config.UnifiedHooks) {
	// PreTool -> PreToolUse
	for _, h := range unified.PreTool {
		w.addHook(settings, "PreToolUse", h)
	}

	// PostTool -> PostToolUse
	for _, h := range unified.PostTool {
		w.addHook(settings, "PostToolUse", h)
	}

	// SessionStart -> SessionStart
	for _, h := range unified.SessionStart {
		w.addHook(settings, "SessionStart", h)
	}

	// SessionEnd -> SessionEnd
	for _, h := range unified.SessionEnd {
		w.addHook(settings, "SessionEnd", h)
	}

	// PreShell -> PreToolUse with Bash matcher
	for _, h := range unified.PreShell {
		hook := h
		if hook.Matcher == "" {
			hook.Matcher = "Bash"
		}
		w.addHook(settings, "PreToolUse", hook)
	}

	// PostFileEdit -> PostToolUse with Edit|Write matcher
	for _, h := range unified.PostFileEdit {
		hook := h
		if hook.Matcher == "" {
			hook.Matcher = "Edit|Write"
		}
		w.addHook(settings, "PostToolUse", hook)
	}
}

// addBackendHooks adds backend-specific passthrough hooks.
func (w *ClaudeCodeHookWriter) addBackendHooks(settings *claudeCodeSettings, backendHooks config.BackendHooks) {
	for eventName, hooks := range backendHooks {
		for _, h := range hooks {
			w.addHook(settings, eventName, h)
		}
	}
}

// addHook adds a single hook to the settings for the given event.
func (w *ClaudeCodeHookWriter) addHook(settings *claudeCodeSettings, eventName string, h config.Hook) {
	ccHook := claudeCodeHook{
		Type:    h.Type,
		Command: h.Command,
		Prompt:  h.Prompt,
		Timeout: h.Timeout,
		Async:   h.Async,
		SCM:     computeHookHash(h),
	}

	// Default type to "command"
	if ccHook.Type == "" {
		ccHook.Type = "command"
	}

	// Find or create matcher entry
	matcher := h.Matcher
	matchers := settings.Hooks[eventName]

	// Look for existing matcher with same pattern
	found := false
	for i, m := range matchers {
		if m.Matcher == matcher {
			matchers[i].Hooks = append(matchers[i].Hooks, ccHook)
			found = true
			break
		}
	}

	if !found {
		matchers = append(matchers, claudeCodeHookMatcher{
			Matcher: matcher,
			Hooks:   []claudeCodeHook{ccHook},
		})
	}

	settings.Hooks[eventName] = matchers
}

// GeminiHookWriter writes hooks to Gemini CLI's settings.json format.
type GeminiHookWriter struct{}

// HooksPath returns the path to Gemini's project-level settings.json file.
func (w *GeminiHookWriter) HooksPath(projectDir string) string {
	return filepath.Join(projectDir, ".gemini", "settings.json")
}

// geminiSettings represents the structure of .gemini/settings.json
type geminiSettings struct {
	Hooks map[string][]geminiHook `json:"hooks,omitempty"`
	// Preserve other settings
	Other map[string]json.RawMessage `json:"-"`
}

// geminiHook represents a single hook in Gemini CLI format.
type geminiHook struct {
	Command string `json:"command,omitempty"`
	SCM     string `json:"_scm,omitempty"` // Hash identifying SCM-managed hooks
}

// WriteHooks implements HookWriter for Gemini CLI.
func (w *GeminiHookWriter) WriteHooks(cfg *config.HooksConfig, projectDir string) error {
	if cfg == nil {
		cfg = &config.HooksConfig{}
	}

	settingsPath := w.HooksPath(projectDir)

	// Ensure .gemini directory exists
	geminiDir := filepath.Dir(settingsPath)
	if err := os.MkdirAll(geminiDir, 0755); err != nil {
		return fmt.Errorf("failed to create .gemini directory: %w", err)
	}

	// Load existing settings
	settings, err := w.loadSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("failed to load existing settings: %w", err)
	}

	// Remove old SCM-managed hooks from settings
	w.removeScmHooks(settings)

	// Add SCM hooks from unified config
	w.addUnifiedHooks(settings, cfg.Unified)

	// Add SCM hooks from backend-specific passthrough
	if backendHooks, ok := cfg.Plugins["gemini"]; ok {
		w.addBackendHooks(settings, backendHooks)
	}

	// Write settings back
	return w.saveSettings(settingsPath, settings)
}

// loadSettings loads existing settings.json or returns empty settings.
func (w *GeminiHookWriter) loadSettings(path string) (*geminiSettings, error) {
	settings := &geminiSettings{
		Hooks: make(map[string][]geminiHook),
		Other: make(map[string]json.RawMessage),
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return settings, nil
		}
		return nil, err
	}

	// First unmarshal to get all fields
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse settings.json: %w", err)
	}

	// Extract hooks separately
	if hooksRaw, ok := raw["hooks"]; ok {
		if err := json.Unmarshal(hooksRaw, &settings.Hooks); err != nil {
			return nil, fmt.Errorf("failed to parse hooks: %w", err)
		}
		delete(raw, "hooks")
	}

	// Preserve other fields
	settings.Other = raw

	return settings, nil
}

// saveSettings writes settings back to settings.json.
func (w *GeminiHookWriter) saveSettings(path string, settings *geminiSettings) error {
	// Build output map starting with preserved fields
	output := make(map[string]interface{})
	for k, v := range settings.Other {
		var val interface{}
		json.Unmarshal(v, &val)
		output[k] = val
	}

	// Add hooks if non-empty
	if len(settings.Hooks) > 0 {
		output["hooks"] = settings.Hooks
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// removeScmHooks removes all hooks with _scm field from settings.
func (w *GeminiHookWriter) removeScmHooks(settings *geminiSettings) {
	for eventName, hooks := range settings.Hooks {
		var filteredHooks []geminiHook
		for _, hook := range hooks {
			if hook.SCM == "" {
				filteredHooks = append(filteredHooks, hook)
			}
		}
		if len(filteredHooks) > 0 {
			settings.Hooks[eventName] = filteredHooks
		} else {
			delete(settings.Hooks, eventName)
		}
	}
}

// addUnifiedHooks translates unified hooks to Gemini CLI format and adds them.
func (w *GeminiHookWriter) addUnifiedHooks(settings *geminiSettings, unified config.UnifiedHooks) {
	// SessionStart -> SessionStart
	for _, h := range unified.SessionStart {
		w.addHook(settings, "SessionStart", h)
	}

	// SessionEnd -> SessionEnd
	for _, h := range unified.SessionEnd {
		w.addHook(settings, "SessionEnd", h)
	}

	// PreTool -> BeforeTool
	for _, h := range unified.PreTool {
		w.addHook(settings, "BeforeTool", h)
	}

	// PostTool -> AfterTool
	for _, h := range unified.PostTool {
		w.addHook(settings, "AfterTool", h)
	}
}

// addBackendHooks adds backend-specific passthrough hooks.
func (w *GeminiHookWriter) addBackendHooks(settings *geminiSettings, backendHooks config.BackendHooks) {
	for eventName, hooks := range backendHooks {
		for _, h := range hooks {
			w.addHook(settings, eventName, h)
		}
	}
}

// addHook adds a single hook to the settings for the given event.
func (w *GeminiHookWriter) addHook(settings *geminiSettings, eventName string, h config.Hook) {
	hook := geminiHook{
		Command: h.Command,
		SCM:     computeHookHash(h),
	}

	settings.Hooks[eventName] = append(settings.Hooks[eventName], hook)
}

// ContextInjectionHook returns a Hook configured for SCM context injection.
// This hook is automatically registered by backends to inject session context.
const ContextInjectionCommand = "scm hook inject-context"

// ContextInjectionTimeout is the timeout for the context injection hook in seconds.
// This ensures Claude doesn't hang waiting for the hook if scm isn't found or fails.
const ContextInjectionTimeout = 5

// NewContextInjectionHook creates a hook for context injection.
func NewContextInjectionHook() config.Hook {
	return config.Hook{
		Command: ContextInjectionCommand,
		Type:    "command",
		Timeout: ContextInjectionTimeout,
	}
}
