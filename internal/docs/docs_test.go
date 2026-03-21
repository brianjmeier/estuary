package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadmeReferencesFeatures(t *testing.T) {
	root := projectRoot(t)
	readmePath := filepath.Join(root, "README.md")
	featuresPath := filepath.Join(root, "FEATURES.md")

	readme, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read readme: %v", err)
	}
	if _, err := os.Stat(featuresPath); err != nil {
		t.Fatalf("features file missing: %v", err)
	}
	if !strings.Contains(string(readme), "FEATURES.md") {
		t.Fatal("README.md must reference FEATURES.md")
	}
}

func projectRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}
