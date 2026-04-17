package mesh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "mesh.json")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return p
}

func TestLoadMeshConfig_DefaultHTTPLoopback(t *testing.T) {
	path := writeConfig(t, `{"listen":"127.0.0.1:7071"}`)
	cfg, err := LoadMeshConfig(path)
	if err != nil {
		t.Fatalf("LoadMeshConfig err: %v", err)
	}
	if cfg.HttpListen != "127.0.0.1:6060" {
		t.Fatalf("expected loopback default http_listen, got %q", cfg.HttpListen)
	}
}

func TestLoadMeshConfig_RejectsDebugHTTPConflict(t *testing.T) {
	path := writeConfig(t, `{"listen":"127.0.0.1:7071","debug":"127.0.0.1:6060","http_listen":"127.0.0.1:6061"}`)
	_, err := LoadMeshConfig(path)
	if err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("expected conflict error, got: %v", err)
	}
}

func TestLoadMeshConfig_RejectsWildcardHTTP(t *testing.T) {
	path := writeConfig(t, `{"listen":"127.0.0.1:7071","http_listen":"0.0.0.0:6060"}`)
	_, err := LoadMeshConfig(path)
	if err == nil || !strings.Contains(err.Error(), "must not bind wildcard") {
		t.Fatalf("expected wildcard bind error, got: %v", err)
	}
}

func TestLoadMeshConfig_RejectsConflictingListenAndHostPort(t *testing.T) {
	path := writeConfig(t, `{"listen":"127.0.0.1:7071","host":"127.0.0.1","port":7071}`)
	_, err := LoadMeshConfig(path)
	if err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("expected conflict error, got: %v", err)
	}
}

func TestLoadMeshConfig_RejectsTLSWhenDisabledWithMaterial(t *testing.T) {
	path := writeConfig(t, `{"listen":"127.0.0.1:7071","http_listen":"127.0.0.1:6060","tls":{"enabled":false,"cert_file":"a.pem","key_file":"b.pem","ca_file":"c.pem"}}`)
	_, err := LoadMeshConfig(path)
	if err == nil || !strings.Contains(err.Error(), "tls.enabled=false") {
		t.Fatalf("expected tls.enabled=false error, got: %v", err)
	}
}

func TestLoadMeshConfig_RejectsMissingTLSFiles(t *testing.T) {
	path := writeConfig(t, `{"listen":"127.0.0.1:7071","http_listen":"127.0.0.1:6060","tls":{"enabled":true,"cert_file":"/no/cert.pem","key_file":"/no/key.pem","ca_file":"/no/ca.pem"}}`)
	_, err := LoadMeshConfig(path)
	if err == nil || !strings.Contains(err.Error(), "not accessible") {
		t.Fatalf("expected inaccessible tls file error, got: %v", err)
	}
}

func TestValidateRuntimePaths_RejectsFilePath(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "not-dir")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := validateRuntimePaths(f, ""); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("expected not-a-directory error, got: %v", err)
	}
}
