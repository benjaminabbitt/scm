package remote

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLockfileManager_LoadEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	manager := NewLockfileManager(tmpDir)

	lockfile, err := manager.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if lockfile.Bundles == nil {
		t.Error("Bundles map should be initialized")
	}
	if lockfile.Profiles == nil {
		t.Error("Profiles map should be initialized")
	}
	if !lockfile.IsEmpty() {
		t.Error("new lockfile should be empty")
	}
}

func TestLockfileManager_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	manager := NewLockfileManager(tmpDir)

	// Create lockfile
	lockfile := &Lockfile{
		Version:  1,
		Bundles:  make(map[string]LockEntry),
		Profiles: make(map[string]LockEntry),
	}

	now := time.Now().UTC().Truncate(time.Second)
	lockfile.AddEntry(ItemTypeBundle, "alice/go-tools", LockEntry{
		SHA:        "abc1234def5678",
		URL:        "https://github.com/alice/scm",
		SCMVersion: "v1",
		FetchedAt:  now,
	})

	// Save
	if err := manager.Save(lockfile); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	// Verify file exists
	path := manager.Path()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("lockfile not created at %s", path)
	}

	// Load
	loaded, err := manager.Load()
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}

	if loaded.Version != 1 {
		t.Errorf("Version = %d, want 1", loaded.Version)
	}

	entry, ok := loaded.GetEntry(ItemTypeBundle, "alice/go-tools")
	if !ok {
		t.Fatal("entry not found")
	}
	if entry.SHA != "abc1234def5678" {
		t.Errorf("SHA = %q, want %q", entry.SHA, "abc1234def5678")
	}
	if entry.URL != "https://github.com/alice/scm" {
		t.Errorf("URL = %q, want %q", entry.URL, "https://github.com/alice/scm")
	}
}

func TestLockfile_AddEntry(t *testing.T) {
	lockfile := &Lockfile{
		Bundles:  make(map[string]LockEntry),
		Profiles: make(map[string]LockEntry),
	}

	entry := LockEntry{SHA: "abc123"}

	lockfile.AddEntry(ItemTypeBundle, "alice/go-tools", entry)
	lockfile.AddEntry(ItemTypeProfile, "alice/secure", entry)

	if len(lockfile.Bundles) != 1 {
		t.Errorf("Bundles count = %d, want 1", len(lockfile.Bundles))
	}
	if len(lockfile.Profiles) != 1 {
		t.Errorf("Profiles count = %d, want 1", len(lockfile.Profiles))
	}
}

func TestLockfile_GetEntry(t *testing.T) {
	lockfile := &Lockfile{
		Bundles: map[string]LockEntry{
			"alice/go-tools": {SHA: "abc123"},
		},
		Profiles: make(map[string]LockEntry),
	}

	// Existing entry
	entry, ok := lockfile.GetEntry(ItemTypeBundle, "alice/go-tools")
	if !ok {
		t.Fatal("expected entry to exist")
	}
	if entry.SHA != "abc123" {
		t.Errorf("SHA = %q, want %q", entry.SHA, "abc123")
	}

	// Non-existing entry
	_, ok = lockfile.GetEntry(ItemTypeBundle, "bob/missing")
	if ok {
		t.Error("expected entry to not exist")
	}
}

func TestLockfile_RemoveEntry(t *testing.T) {
	lockfile := &Lockfile{
		Bundles: map[string]LockEntry{
			"alice/go-tools": {SHA: "abc123"},
			"bob/testing":    {SHA: "def456"},
		},
		Profiles: make(map[string]LockEntry),
	}

	lockfile.RemoveEntry(ItemTypeBundle, "alice/go-tools")

	if len(lockfile.Bundles) != 1 {
		t.Errorf("Bundles count = %d, want 1", len(lockfile.Bundles))
	}
	if _, ok := lockfile.GetEntry(ItemTypeBundle, "alice/go-tools"); ok {
		t.Error("entry should have been removed")
	}
	if _, ok := lockfile.GetEntry(ItemTypeBundle, "bob/testing"); !ok {
		t.Error("other entry should still exist")
	}
}

func TestLockfile_AllEntries(t *testing.T) {
	lockfile := &Lockfile{
		Bundles: map[string]LockEntry{
			"alice/go-tools": {SHA: "abc123"},
		},
		Profiles: map[string]LockEntry{
			"alice/secure": {SHA: "ghi789"},
		},
	}

	entries := lockfile.AllEntries()
	if len(entries) != 2 {
		t.Errorf("entries count = %d, want 2", len(entries))
	}

	// Verify each type is present
	typeCount := make(map[ItemType]int)
	for _, e := range entries {
		typeCount[e.Type]++
	}

	if typeCount[ItemTypeBundle] != 1 {
		t.Errorf("bundle count = %d, want 1", typeCount[ItemTypeBundle])
	}
	if typeCount[ItemTypeProfile] != 1 {
		t.Errorf("profile count = %d, want 1", typeCount[ItemTypeProfile])
	}
}

func TestLockfile_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		lockfile Lockfile
		want     bool
	}{
		{
			name: "empty",
			lockfile: Lockfile{
				Bundles:  make(map[string]LockEntry),
				Profiles: make(map[string]LockEntry),
			},
			want: true,
		},
		{
			name: "with bundle",
			lockfile: Lockfile{
				Bundles:  map[string]LockEntry{"a": {}},
				Profiles: make(map[string]LockEntry),
			},
			want: false,
		},
		{
			name: "with profile",
			lockfile: Lockfile{
				Bundles:  make(map[string]LockEntry),
				Profiles: map[string]LockEntry{"a": {}},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.lockfile.IsEmpty(); got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLockfile_Count(t *testing.T) {
	lockfile := Lockfile{
		Bundles:  map[string]LockEntry{"a": {}, "b": {}},
		Profiles: map[string]LockEntry{"d": {}, "e": {}, "f": {}},
	}

	if got := lockfile.Count(); got != 5 {
		t.Errorf("Count() = %d, want 5", got)
	}
}

func TestLockfileManager_Path(t *testing.T) {
	manager := NewLockfileManager("/home/user/.scm")
	path := manager.Path()
	expected := filepath.Join("/home/user/.scm", "lock.yaml")

	if path != expected {
		t.Errorf("Path() = %q, want %q", path, expected)
	}
}

func TestLockfileManager_DefaultDir(t *testing.T) {
	manager := NewLockfileManager("")
	path := manager.Path()
	expected := filepath.Join(".scm", "lock.yaml")

	if path != expected {
		t.Errorf("Path() = %q, want %q", path, expected)
	}
}
