package passwordhash

import (
	"errors"
	"strings"
	"testing"
)

func TestHashAndVerify(t *testing.T) {
	encoded, err := Hash("a long Web passphrase")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if strings.Contains(encoded, "a long Web passphrase") || !Valid(encoded) {
		t.Fatalf("encoded verifier is unsafe or invalid: %q", encoded)
	}
	if ok, err := Verify(encoded, "a long Web passphrase"); err != nil || !ok {
		t.Fatalf("Verify correct password = %v, %v", ok, err)
	}
	if ok, err := Verify(encoded, "wrong password"); err != nil || ok {
		t.Fatalf("Verify wrong password = %v, %v", ok, err)
	}
}

func TestVerifyRejectsUnsafeParametersBeforeHashing(t *testing.T) {
	unsafe := "$argon2id$v=19$m=1048576,t=3,p=1$MDEyMzQ1Njc4OWFiY2RlZg$MDEyMzQ1Njc4OWFiY2RlZg"
	if ok, err := Verify(unsafe, "password"); err == nil || ok {
		t.Fatalf("Verify unsafe parameters = %v, %v", ok, err)
	}
}

func TestValidRejectsMalformedHashes(t *testing.T) {
	validSalt := "MDEyMzQ1Njc4OWFiY2RlZg"
	validKey := "MDEyMzQ1Njc4OWFiY2RlZg"
	tests := []string{
		"",
		"$argon2i$v=19$m=65536,t=3,p=1$" + validSalt + "$" + validKey,
		"$argon2id$v=18$m=65536,t=3,p=1$" + validSalt + "$" + validKey,
		"$argon2id$v=bad$m=65536,t=3,p=1$" + validSalt + "$" + validKey,
		"$argon2id$v=19$bad-params$" + validSalt + "$" + validKey,
		"$argon2id$v=19$m=1024,t=3,p=1$" + validSalt + "$" + validKey,
		"$argon2id$v=19$m=65536,t=1,p=1$" + validSalt + "$" + validKey,
		"$argon2id$v=19$m=65536,t=3,p=0$" + validSalt + "$" + validKey,
		"$argon2id$v=19$m=65536,t=3,p=1$%%%$" + validKey,
		"$argon2id$v=19$m=65536,t=3,p=1$YWJj$" + validKey,
		"$argon2id$v=19$m=65536,t=3,p=1$" + validSalt + "$%%%",
		"$argon2id$v=19$m=65536,t=3,p=1$" + validSalt + "$YWJj",
	}
	for _, encoded := range tests {
		if Valid(encoded) {
			t.Fatalf("Valid(%q) = true", encoded)
		}
		if ok, err := Verify(encoded, "password"); ok || !errors.Is(err, ErrInvalidHash) {
			t.Fatalf("Verify malformed hash = %v, %v", ok, err)
		}
	}
}
