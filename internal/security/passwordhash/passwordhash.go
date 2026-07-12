// Package passwordhash provides the one-way verifier used by optional Web access.
package passwordhash

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	memoryKiB   = 64 * 1024
	iterations  = 3
	parallelism = 1
	saltBytes   = 16
	keyBytes    = 32
)

var ErrInvalidHash = errors.New("invalid Argon2id password hash")

type parameters struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
}

// Hash returns a PHC-formatted Argon2id verifier with a fresh random salt.
func Hash(password string) (string, error) {
	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, iterations, memoryKiB, parallelism, keyBytes)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		memoryKiB,
		iterations,
		parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// Verify compares password with an encoded verifier. Encoded parameters are
// bounded before Argon2 is invoked so a corrupt settings file cannot request
// excessive CPU or memory.
func Verify(encoded string, password string) (bool, error) {
	params, salt, expected, err := decode(encoded)
	if err != nil {
		return false, err
	}
	actual := argon2.IDKey([]byte(password), salt, params.iterations, params.memory, params.parallelism, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

// Valid reports whether encoded is a supported, safely bounded verifier.
func Valid(encoded string) bool {
	_, _, _, err := decode(encoded)
	return err == nil
}

func decode(encoded string) (parameters, []byte, []byte, error) {
	parts := strings.Split(strings.TrimSpace(encoded), "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return parameters{}, nil, nil, ErrInvalidHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return parameters{}, nil, nil, ErrInvalidHash
	}
	var params parameters
	var parallelismValue uint32
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params.memory, &params.iterations, &parallelismValue); err != nil {
		return parameters{}, nil, nil, ErrInvalidHash
	}
	if params.memory < 19*1024 || params.memory > 128*1024 ||
		params.iterations < 2 || params.iterations > 5 ||
		parallelismValue < 1 || parallelismValue > 4 {
		return parameters{}, nil, nil, ErrInvalidHash
	}
	params.parallelism = uint8(parallelismValue)
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 16 || len(salt) > 64 {
		return parameters{}, nil, nil, ErrInvalidHash
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) < 16 || len(expected) > 64 {
		return parameters{}, nil, nil, ErrInvalidHash
	}
	return params, salt, expected, nil
}
