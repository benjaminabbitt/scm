package remote

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// VendorManager handles vendoring remote dependencies locally.
type VendorManager struct {
	baseDir    string
	configPath string
}

// NewVendorManager creates a new vendor manager.
func NewVendorManager(baseDir string) *VendorManager {
	if baseDir == "" {
		baseDir = ".scm"
	}
	return &VendorManager{
		baseDir: baseDir,
	}
}

// VendorDir returns the vendor directory path.
func (m *VendorManager) VendorDir() string {
	return filepath.Join(m.baseDir, "vendor")
}

// IsVendored checks if vendor mode is enabled.
func (m *VendorManager) IsVendored() bool {
	configPath := filepath.Join(".scm", "remotes.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return false
	}

	var cfg struct {
		Vendor bool `yaml:"vendor"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return false
	}

	return cfg.Vendor
}

// SetVendorMode enables or disables vendor mode.
func (m *VendorManager) SetVendorMode(enabled bool) error {
	configPath := filepath.Join(".scm", "remotes.yaml")

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}

	var existingRaw map[string]interface{}
	data, err := os.ReadFile(configPath)
	if err == nil {
		yaml.Unmarshal(data, &existingRaw)
	}
	if existingRaw == nil {
		existingRaw = make(map[string]interface{})
	}

	if enabled {
		existingRaw["vendor"] = true
	} else {
		delete(existingRaw, "vendor")
	}

	out, err := yaml.Marshal(existingRaw)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, out, 0644)
}

// VendorAll copies all locked dependencies to the vendor directory.
func (m *VendorManager) VendorAll(ctx context.Context, lockfile *Lockfile, registry *Registry, auth AuthConfig) error {
	vendorDir := m.VendorDir()

	// Clean existing vendor directory
	if err := os.RemoveAll(vendorDir); err != nil {
		return fmt.Errorf("failed to clean vendor directory: %w", err)
	}

	entries := lockfile.AllEntries()
	if len(entries) == 0 {
		return fmt.Errorf("no entries in lockfile")
	}

	for _, e := range entries {
		ref, err := ParseReference(e.Ref)
		if err != nil {
			return fmt.Errorf("invalid reference %s: %w", e.Ref, err)
		}

		rem, err := registry.Get(ref.Remote)
		if err != nil {
			return fmt.Errorf("remote not found %s: %w", ref.Remote, err)
		}

		fetcher, err := NewFetcher(rem.URL, auth)
		if err != nil {
			return fmt.Errorf("failed to create fetcher: %w", err)
		}

		owner, repo, err := ParseRepoURL(rem.URL)
		if err != nil {
			return fmt.Errorf("invalid URL: %w", err)
		}

		// Build file path
		filePath := ref.BuildFilePath(e.Type, rem.Version)

		// Fetch content at locked SHA
		content, err := fetcher.FetchFile(ctx, owner, repo, filePath, e.Entry.SHA)
		if err != nil {
			return fmt.Errorf("failed to fetch %s: %w", e.Ref, err)
		}

		// Write to vendor directory
		vendorPath := filepath.Join(vendorDir, string(e.Type)+"s", ref.Remote, ref.Path+".yaml")
		if err := os.MkdirAll(filepath.Dir(vendorPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		if err := os.WriteFile(vendorPath, content, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", vendorPath, err)
		}
	}

	return nil
}

// GetVendored returns content from the vendor directory if available.
func (m *VendorManager) GetVendored(itemType ItemType, ref *Reference) ([]byte, error) {
	vendorPath := filepath.Join(m.VendorDir(), itemType.DirName(), ref.Remote, ref.Path+".yaml")
	return os.ReadFile(vendorPath)
}

// HasVendored checks if an item exists in the vendor directory.
func (m *VendorManager) HasVendored(itemType ItemType, ref *Reference) bool {
	vendorPath := filepath.Join(m.VendorDir(), itemType.DirName(), ref.Remote, ref.Path+".yaml")
	_, err := os.Stat(vendorPath)
	return err == nil
}
