package backends

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// SCMContextDir is the directory for SCM-managed files
	SCMContextDir = ".scm"
	// SCMContextSubdir is the subdirectory for context files
	SCMContextSubdir = ".scm/context"
	// SCMContextFileEnv is the environment variable containing the context file path
	SCMContextFileEnv = "SCM_CONTEXT_FILE"
)

// WriteContextFile writes the assembled context to a hashed filename in .scm/context/.
// Returns the hash (used as filename without .md extension).
// workDir is the directory where the .scm/ directory exists.
func WriteContextFile(workDir string, fragments []*Fragment) (string, error) {
	// Assemble the context content
	var parts []string
	for _, f := range fragments {
		if f.Content == "" {
			continue
		}
		parts = append(parts, strings.TrimSpace(f.Content))
	}

	if len(parts) == 0 {
		// No content - nothing to write
		return "", nil
	}

	content := strings.Join(parts, "\n\n---\n\n")

	// Generate hash-based filename from content
	hash := sha256.Sum256([]byte(content))
	hashStr := hex.EncodeToString(hash[:8]) // First 8 bytes = 16 hex chars

	// Ensure .scm/context directory exists
	contextDir := filepath.Join(workDir, SCMContextSubdir)
	if err := os.MkdirAll(contextDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create %s directory: %w", SCMContextSubdir, err)
	}

	// Write context file
	contextPath := filepath.Join(contextDir, hashStr+".md")
	if err := os.WriteFile(contextPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write context file: %w", err)
	}

	return hashStr, nil
}

// ReadContextFile reads the context file for the given hash from .scm/context/[hash].md.
// Returns empty string if file doesn't exist.
func ReadContextFile(workDir, hash string) (string, error) {
	contextPath := filepath.Join(workDir, SCMContextSubdir, hash+".md")
	content, err := os.ReadFile(contextPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read context file: %w", err)
	}
	return string(content), nil
}

// ReadContextFileAndDelete reads the context file specified by SCM_CONTEXT_FILE env var,
// then deletes the file. Returns empty string if env var not set or file doesn't exist.
func ReadContextFileAndDelete(workDir string) (string, error) {
	contextPath := os.Getenv(SCMContextFileEnv)
	if contextPath == "" {
		return "", nil
	}

	// If relative path, resolve against workDir
	if !filepath.IsAbs(contextPath) {
		contextPath = filepath.Join(workDir, contextPath)
	}

	content, err := os.ReadFile(contextPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read context file: %w", err)
	}

	// Clean up after reading
	if err := os.Remove(contextPath); err != nil {
		// Log but don't fail - content was read successfully
		fmt.Fprintf(os.Stderr, "warning: failed to delete context file %s: %v\n", contextPath, err)
	}

	return string(content), nil
}

