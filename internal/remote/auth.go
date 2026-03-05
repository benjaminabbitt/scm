package remote

import (
	"os"
)

// LoadAuth loads authentication from environment variables.
// Supported variables:
//   - GITHUB_TOKEN or GH_TOKEN for GitHub
//   - GITLAB_TOKEN or GL_TOKEN for GitLab
func LoadAuth(configPath string) AuthConfig {
	auth := AuthConfig{}

	// GitHub token
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		auth.GitHub = token
	}
	if token := os.Getenv("GH_TOKEN"); token != "" {
		auth.GitHub = token
	}

	// GitLab token
	if token := os.Getenv("GITLAB_TOKEN"); token != "" {
		auth.GitLab = token
	}
	if token := os.Getenv("GL_TOKEN"); token != "" {
		auth.GitLab = token
	}

	return auth
}

// HasGitHubAuth returns true if GitHub authentication is configured.
func (a AuthConfig) HasGitHubAuth() bool {
	return a.GitHub != ""
}

// HasGitLabAuth returns true if GitLab authentication is configured.
func (a AuthConfig) HasGitLabAuth() bool {
	return a.GitLab != ""
}

// TokenForForge returns the authentication token for the given forge.
func (a AuthConfig) TokenForForge(forge ForgeType) string {
	switch forge {
	case ForgeGitHub:
		return a.GitHub
	case ForgeGitLab:
		return a.GitLab
	default:
		return ""
	}
}
