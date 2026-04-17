package mesh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateTLSFile_RejectsPermissiveKey(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "node.key")
	if err := os.WriteFile(key, []byte("dummy"), 0o644); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if err := validateTLSFile("tls.key_file", key, true); err == nil || !strings.Contains(err.Error(), "too permissive") {
		t.Fatalf("expected permissive mode error, got: %v", err)
	}
}
