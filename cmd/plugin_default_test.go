package cmd

import (
	"testing"

	"github.com/benjaminabbitt/scm/internal/config"
)

func TestIsKnownPlugin_BuiltIn(t *testing.T) {
	cfg := &config.Config{}
	if !isKnownPlugin(cfg, "claude-code") {
		t.Error("expected claude-code to be known")
	}
	if !isKnownPlugin(cfg, "gemini") {
		t.Error("expected gemini to be known")
	}
}

func TestIsKnownPlugin_Unknown(t *testing.T) {
	cfg := &config.Config{}
	if isKnownPlugin(cfg, "nonexistent-plugin") {
		t.Error("expected nonexistent-plugin to be unknown")
	}
}

func TestAvailablePluginNames_IncludesBuiltIns(t *testing.T) {
	cfg := &config.Config{}
	names := availablePluginNames(cfg)

	expected := map[string]bool{
		"claude-code": false,
		"gemini":      false,
		"aider":       false,
	}

	for _, name := range names {
		if _, ok := expected[name]; ok {
			expected[name] = true
		}
	}

	for name, found := range expected {
		if !found {
			t.Errorf("expected %s in available plugin names", name)
		}
	}
}

func TestAvailablePluginNames_Sorted(t *testing.T) {
	cfg := &config.Config{}
	names := availablePluginNames(cfg)

	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("expected sorted names, but %q < %q at index %d", names[i], names[i-1], i)
		}
	}
}
