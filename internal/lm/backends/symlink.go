package backends

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	// SCMBinDir is the directory for SCM-managed binaries/symlinks
	SCMBinDir = ".scm/bin"
	// SCMBinaryName is the name of the scm binary symlink
	SCMBinaryName = "scm"
)

// EnsureSCMSymlink creates a symlink to the current scm binary at .scm/bin/scm.
// This allows hooks to call scm without requiring it to be in PATH.
// workDir is the directory where the .scm/ directory exists.
func EnsureSCMSymlink(workDir string) (string, error) {
	// Get the path to the currently running scm binary
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve symlinks to get the real path
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve executable path: %w", err)
	}

	// Ensure .scm/bin directory exists
	binDir := filepath.Join(workDir, SCMBinDir)
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create %s directory: %w", SCMBinDir, err)
	}

	// Create/update symlink
	symlinkPath := filepath.Join(binDir, SCMBinaryName)

	// Remove existing symlink if it exists
	if _, err := os.Lstat(symlinkPath); err == nil {
		if err := os.Remove(symlinkPath); err != nil {
			return "", fmt.Errorf("failed to remove existing symlink: %w", err)
		}
	}

	// Create new symlink
	if err := os.Symlink(execPath, symlinkPath); err != nil {
		return "", fmt.Errorf("failed to create symlink: %w", err)
	}

	return symlinkPath, nil
}

// GetContextInjectionCommand returns the hook command for context injection.
// Uses ${CLAUDE_PROJECT_DIR} for Claude Code's variable expansion.
func GetContextInjectionCommand(hash string) string {
	// Claude Code expands ${VAR} syntax in hook commands
	return fmt.Sprintf(`${CLAUDE_PROJECT_DIR}/%s/%s hook inject-context %s`, SCMBinDir, SCMBinaryName, hash)
}

// GetSCMMCPCommand returns the command (executable path) for the SCM MCP server.
// Uses relative path since MCP commands run from the project directory.
// Note: The "mcp" subcommand should be passed via args, not baked into the command.
func GetSCMMCPCommand() string {
	// Use relative path - MCP servers run from project directory
	return fmt.Sprintf(`./%s/%s`, SCMBinDir, SCMBinaryName)
}

// GetSCMMCPArgs returns the arguments for the SCM MCP server.
func GetSCMMCPArgs() []string {
	return []string{"mcp"}
}
