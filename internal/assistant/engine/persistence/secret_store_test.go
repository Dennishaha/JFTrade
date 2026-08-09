package persistence

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecretStoreFileBoundaries(t *testing.T) {
	badSecret := secretStore{path: filepath.Join(t.TempDir(), "bad.json")}
	if err := os.WriteFile(badSecret.path, []byte("{"), 0o600); err != nil {
		t.Fatalf("write bad secret: %v", err)
	}
	if _, err := badSecret.read(); err == nil {
		t.Fatal("secretStore read invalid JSON err = nil, want error")
	}
	if _, _, err := badSecret.get("provider"); err == nil {
		t.Fatal("secretStore get invalid JSON err = nil, want error")
	}
	if err := badSecret.set("provider", "sk"); err == nil {
		t.Fatal("secretStore set invalid JSON err = nil, want error")
	}
	if err := badSecret.delete("provider"); err == nil {
		t.Fatal("secretStore delete invalid JSON err = nil, want error")
	}

	blankSecret := secretStore{path: filepath.Join(t.TempDir(), "blank.json")}
	if err := os.WriteFile(blankSecret.path, []byte(" \n\t "), 0o600); err != nil {
		t.Fatalf("write blank secret: %v", err)
	}
	if data, err := blankSecret.read(); err != nil || len(data) != 0 {
		t.Fatalf("secretStore blank read = %#v/%v, want empty/nil", data, err)
	}

	blocker := filepath.Join(t.TempDir(), "secret-dir-file")
	if err := os.WriteFile(blocker, []byte("file"), 0o600); err != nil {
		t.Fatalf("write secret write blocker: %v", err)
	}
	blockedSecret := secretStore{path: filepath.Join(blocker, "adk.json")}
	if err := blockedSecret.write(map[string]string{"provider": "sk"}); err == nil {
		t.Fatal("secretStore write mkdir err = nil, want error")
	}

	invalidPath := secretStore{path: string([]byte{0})}
	if err := invalidPath.write(map[string]string{"provider": "sk"}); err == nil {
		t.Fatal("secretStore write accepted invalid path")
	}
}
