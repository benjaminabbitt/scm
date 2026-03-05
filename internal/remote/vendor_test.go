package remote

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVendorManager_VendorDir(t *testing.T) {
	manager := NewVendorManager(".scm")
	expected := filepath.Join(".scm", "vendor")

	if got := manager.VendorDir(); got != expected {
		t.Errorf("VendorDir() = %q, want %q", got, expected)
	}
}

func TestVendorManager_DefaultBaseDir(t *testing.T) {
	manager := NewVendorManager("")
	expected := filepath.Join(".scm", "vendor")

	if got := manager.VendorDir(); got != expected {
		t.Errorf("VendorDir() = %q, want %q", got, expected)
	}
}

func TestVendorManager_HasVendored(t *testing.T) {
	tmpDir := t.TempDir()
	manager := NewVendorManager(tmpDir)

	ref := &Reference{
		Remote: "alice",
		Path:   "security",
	}

	// Initially not vendored
	if manager.HasVendored(ItemTypeBundle, ref) {
		t.Error("expected HasVendored to return false initially")
	}

	// Create vendored file
	vendorPath := filepath.Join(manager.VendorDir(), "bundles", "alice", "security.yaml")
	if err := os.MkdirAll(filepath.Dir(vendorPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(vendorPath, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Now should be vendored
	if !manager.HasVendored(ItemTypeBundle, ref) {
		t.Error("expected HasVendored to return true after creating file")
	}
}

func TestVendorManager_GetVendored(t *testing.T) {
	tmpDir := t.TempDir()
	manager := NewVendorManager(tmpDir)

	ref := &Reference{
		Remote: "alice",
		Path:   "security",
	}

	content := []byte("vendored content")

	// Create vendored file
	vendorPath := filepath.Join(manager.VendorDir(), "bundles", "alice", "security.yaml")
	os.MkdirAll(filepath.Dir(vendorPath), 0755)
	os.WriteFile(vendorPath, content, 0644)

	// Get vendored content
	got, err := manager.GetVendored(ItemTypeBundle, ref)
	if err != nil {
		t.Fatalf("GetVendored failed: %v", err)
	}

	if string(got) != string(content) {
		t.Errorf("content = %q, want %q", string(got), string(content))
	}

	// Get non-existent
	badRef := &Reference{Remote: "bob", Path: "other"}
	_, err = manager.GetVendored(ItemTypeBundle, badRef)
	if err == nil {
		t.Error("expected error getting non-vendored file")
	}
}

func TestVendorManager_NestedPath(t *testing.T) {
	tmpDir := t.TempDir()
	manager := NewVendorManager(tmpDir)

	ref := &Reference{
		Remote: "alice",
		Path:   "lang/go/testing",
	}

	content := []byte("nested vendored content")

	// Create vendored file with nested path
	vendorPath := filepath.Join(manager.VendorDir(), "bundles", "alice", "lang", "go", "testing.yaml")
	os.MkdirAll(filepath.Dir(vendorPath), 0755)
	os.WriteFile(vendorPath, content, 0644)

	if !manager.HasVendored(ItemTypeBundle, ref) {
		t.Error("expected HasVendored to return true for nested path")
	}

	got, err := manager.GetVendored(ItemTypeBundle, ref)
	if err != nil {
		t.Fatalf("GetVendored failed: %v", err)
	}

	if string(got) != string(content) {
		t.Errorf("content = %q, want %q", string(got), string(content))
	}
}

func TestVendorManager_DifferentItemTypes(t *testing.T) {
	tmpDir := t.TempDir()
	manager := NewVendorManager(tmpDir)

	ref := &Reference{
		Remote: "alice",
		Path:   "test",
	}

	// Create files for different types (bundles and profiles only)
	for _, itemType := range []ItemType{ItemTypeBundle, ItemTypeProfile} {
		vendorPath := filepath.Join(manager.VendorDir(), itemType.DirName(), "alice", "test.yaml")
		os.MkdirAll(filepath.Dir(vendorPath), 0755)
		os.WriteFile(vendorPath, []byte(string(itemType)), 0644)
	}

	// Verify each type is found correctly
	for _, itemType := range []ItemType{ItemTypeBundle, ItemTypeProfile} {
		if !manager.HasVendored(itemType, ref) {
			t.Errorf("expected HasVendored to return true for %s", itemType)
		}

		content, err := manager.GetVendored(itemType, ref)
		if err != nil {
			t.Errorf("GetVendored(%s) failed: %v", itemType, err)
		}

		if string(content) != string(itemType) {
			t.Errorf("content for %s = %q, want %q", itemType, string(content), string(itemType))
		}
	}
}
