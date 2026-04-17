package mesh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeMeshConfigForTest(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mesh.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadMeshConfigRejectsListenAndHostPortTogether(t *testing.T) {
	path := writeMeshConfigForTest(t, `{
		"node_id":"node1",
		"listen":"127.0.0.1:7072",
		"host":"127.0.0.1",
		"port":7072,
		"http_listen":"127.0.0.1:6060"
	}`)

	_, err := LoadMeshConfig(path)
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutually exclusive listen/host+port error, got %v", err)
	}
}

func TestLoadMeshConfigRejectsLegacyDebugField(t *testing.T) {
	path := writeMeshConfigForTest(t, `{
		"node_id":"node1",
		"listen":"127.0.0.1:7072",
		"http_listen":"127.0.0.1:6060",
		"debug":"127.0.0.1:6061"
	}`)

	_, err := LoadMeshConfig(path)
	if err == nil || !strings.Contains(err.Error(), "legacy debug field") {
		t.Fatalf("expected legacy debug field rejection, got %v", err)
	}
}

func TestLoadMeshConfigRejectsNonLoopbackAdminSurface(t *testing.T) {
	path := writeMeshConfigForTest(t, `{
		"node_id":"node1",
		"listen":"127.0.0.1:7072",
		"http_listen":":6060",
		"admin_endpoints_enabled":true
	}`)

	_, err := LoadMeshConfig(path)
	if err == nil || !strings.Contains(err.Error(), "admin_endpoints_enabled requires loopback-only http_listen") {
		t.Fatalf("expected admin surface rejection, got %v", err)
	}
}

func TestValidateRuntimePathsRejectsFilePath(t *testing.T) {
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "data-file")
	if err := os.WriteFile(dataPath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	err := validateRuntimePaths(dataPath, "")
	if err == nil || !strings.Contains(err.Error(), "data_dir path must be a directory") {
		t.Fatalf("expected data_dir file rejection, got %v", err)
	}
}
