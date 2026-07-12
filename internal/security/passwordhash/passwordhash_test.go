package passwordhash

import (
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
