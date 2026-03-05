package remote

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsURLReference(t *testing.T) {
	tests := []struct {
		ref  string
		want bool
	}{
		{"https://github.com/owner/repo@v1/bundles/name", true},
		{"http://github.com/owner/repo@v1/bundles/name", true},
		{"git@github.com:owner/repo@v1/bundles/name", true},
		{"file:///path/to/repo@v1/bundles/name", true},
		{"local-bundle", false},
		{"remote/bundle", false},
		{"alice/security", false},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			if got := IsURLReference(tt.ref); got != tt.want {
				t.Errorf("IsURLReference(%q) = %v, want %v", tt.ref, got, tt.want)
			}
		})
	}
}

func TestExtractBundleName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{".scm/bundles/github.com/owner/repo/core-practices.yaml", "core-practices"},
		{"/home/user/.scm/bundles/testing.yaml", "testing"},
		{"bundle.yaml", "bundle"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := ExtractBundleName(tt.path); got != tt.want {
				t.Errorf("ExtractBundleName(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestBundleResolver_ResolveToLocalPath(t *testing.T) {
	// Create temp directory with a bundle file
	tmpDir := t.TempDir()
	scmDir := filepath.Join(tmpDir, ".scm")
	bundleDir := filepath.Join(scmDir, "bundles", "github.com", "owner", "repo")
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		t.Fatal(err)
	}

	bundlePath := filepath.Join(bundleDir, "core-practices.yaml")
	if err := os.WriteFile(bundlePath, []byte("version: '1.0.0'\n"), 0644); err != nil {
		t.Fatal(err)
	}

	resolver := NewBundleResolver(scmDir)

	tests := []struct {
		name       string
		bundleRef  string
		wantPath   string
		wantErr    bool
	}{
		{
			name:      "valid https reference",
			bundleRef: "https://github.com/owner/repo@v1/bundles/core-practices",
			wantPath:  bundlePath,
			wantErr:   false,
		},
		{
			name:      "bundle not found",
			bundleRef: "https://github.com/owner/repo@v1/bundles/nonexistent",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolver.ResolveToLocalPath(tt.bundleRef)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveToLocalPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.wantPath {
				t.Errorf("ResolveToLocalPath() = %q, want %q", got, tt.wantPath)
			}
		})
	}
}
