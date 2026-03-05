package remote

import (
	"testing"
)

func TestParseProfileRefs(t *testing.T) {
	tests := []struct {
		name     string
		refs     []string
		itemType ItemType
		want     int // Number of remote refs
	}{
		{
			name:     "all local",
			refs:     []string{"security", "golang"},
			itemType: ItemTypeBundle,
			want:     0,
		},
		{
			name:     "all remote",
			refs:     []string{"alice/security", "bob/golang"},
			itemType: ItemTypeBundle,
			want:     2,
		},
		{
			name:     "mixed",
			refs:     []string{"local", "alice/remote", "another-local"},
			itemType: ItemTypeProfile,
			want:     1,
		},
		{
			name:     "with version",
			refs:     []string{"alice/security@v1.0.0", "bob/golang@main"},
			itemType: ItemTypeBundle,
			want:     2,
		},
		{
			name:     "nested path",
			refs:     []string{"alice/lang/go/testing"},
			itemType: ItemTypeBundle,
			want:     1,
		},
		{
			name:     "empty",
			refs:     []string{},
			itemType: ItemTypeBundle,
			want:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseProfileRefs(tt.refs, tt.itemType)
			if len(got) != tt.want {
				t.Errorf("ParseProfileRefs() returned %d refs, want %d", len(got), tt.want)
			}

			// Verify item type is set correctly
			for _, ref := range got {
				if ref.ItemType != tt.itemType {
					t.Errorf("ItemType = %v, want %v", ref.ItemType, tt.itemType)
				}
			}
		})
	}
}

func TestParseProfileRefs_RefFormat(t *testing.T) {
	refs := []string{"alice/go-tools@v1.0.0"}
	results := ParseProfileRefs(refs, ItemTypeBundle)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Ref != "alice/go-tools@v1.0.0" {
		t.Errorf("Ref = %q, want %q", results[0].Ref, "alice/go-tools@v1.0.0")
	}

	if results[0].Cached {
		t.Error("new ref should not be cached")
	}
}

func TestRemoteRef_Fields(t *testing.T) {
	ref := RemoteRef{
		Ref:      "alice/go-tools",
		ItemType: ItemTypeBundle,
		Cached:   false,
	}

	if ref.Ref != "alice/go-tools" {
		t.Errorf("Ref = %q, want %q", ref.Ref, "alice/go-tools")
	}
	if ref.ItemType != ItemTypeBundle {
		t.Errorf("ItemType = %v, want %v", ref.ItemType, ItemTypeBundle)
	}
	if ref.Cached {
		t.Error("Cached should be false")
	}
}
