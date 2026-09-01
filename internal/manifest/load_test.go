package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMultiDocumentYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "objects.yaml")
	content := "apiVersion: v1\nkind: Namespace\nmetadata:\n  name: demo\n---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: settings\n  namespace: demo\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	objects, err := Load([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 2 {
		t.Fatalf("got %d objects, want 2", len(objects))
	}
}
