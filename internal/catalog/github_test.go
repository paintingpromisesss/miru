package catalog

import (
	"testing"
	"time"
)

func TestCatalogLiveFetch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live network test in short mode")
	}

	cat := NewCatalog("MetaCubeX/meta-rules-dat", "meta", "", 1*time.Hour, "")
	files, err := cat.fetchTree()
	if err != nil {
		t.Logf("fetchTree error: %v", err)
		return
	}

	if len(files) == 0 {
		t.Fatalf("expected files from catalog, got 0")
	}

	foundGoogle := false
	foundYoutube := false
	for _, f := range files {
		if f.Name == "google" {
			foundGoogle = true
		}
		if f.Name == "youtube" {
			foundYoutube = true
		}
	}

	if !foundGoogle {
		t.Errorf("expected to find 'google' in catalog, but not found among %d rules", len(files))
	}
	if !foundYoutube {
		t.Errorf("expected to find 'youtube' in catalog, but not found among %d rules", len(files))
	}
	t.Logf("Successfully fetched %d rulesets (found google: %v, youtube: %v)", len(files), foundGoogle, foundYoutube)
}
