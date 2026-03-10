package backends

import (
	"fmt"
	"os"
	"path/filepath"
)

// cachedExecPath stores the resolved executable path (set once at startup).
var cachedExecPath string

// GetExecutablePath returns the absolute path to the current scm binary.
// The path is resolved once and cached for the lifetime of the process.
func GetExecutablePath() (string, error) {
	if cachedExecPath != "" {
		return cachedExecPath, nil
	}

	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve symlinks to get the real path
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve executable path: %w", err)
	}

	cachedExecPath = execPath
	return execPath, nil
}

// SetExecutablePathForTesting allows tests to override the executable path.
func SetExecutablePathForTesting(path string) {
	cachedExecPath = path
}

// GetContextInjectionCommand returns the hook command for context injection.
// Uses absolute path to the current scm binary.
func GetContextInjectionCommand(hash string) string {
	execPath, err := GetExecutablePath()
	if err != nil {
		// Fallback to "scm" if we can't get the path (shouldn't happen)
		execPath = "scm"
	}
	return fmt.Sprintf(`%s hook inject-context %s`, execPath, hash)
}

// GetSCMMCPCommand returns the command (executable path) for the SCM MCP server.
// Uses absolute path to the current scm binary.
func GetSCMMCPCommand() string {
	execPath, err := GetExecutablePath()
	if err != nil {
		// Fallback to "scm" if we can't get the path (shouldn't happen)
		return "scm"
	}
	return execPath
}

// GetSCMMCPArgs returns the arguments for the SCM MCP server.
func GetSCMMCPArgs() []string {
	return []string{"mcp"}
}
