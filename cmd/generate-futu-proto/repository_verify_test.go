package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepositoryDigestDetectsGeneratedOutputDrift(t *testing.T) {
	fixture := newFutuFixture(t)
	protoDirectory := filepath.Join(fixture.root, "pkg", "futu", "proto")
	pbDirectory := filepath.Join(fixture.root, "pkg", "futu", "pb")
	require.NoError(t, os.Remove(filepath.Join(protoDirectory, "existing.proto")))
	require.NoError(t, os.Remove(filepath.Join(pbDirectory, "existing.pb.go")))
	require.NoError(t, os.WriteFile(
		filepath.Join(protoDirectory, "Common.proto"),
		[]byte("syntax = \"proto3\";\npackage Common;\n"),
		0o600,
	))
	require.NoError(t, os.MkdirAll(filepath.Join(pbDirectory, "common"), 0o755))
	pbPath := filepath.Join(pbDirectory, "common", "Common.pb.go")
	require.NoError(t, os.WriteFile(pbPath, []byte("// Code generated. DO NOT EDIT.\npackage common\n"), 0o600))

	digest, err := inspectFutuRepository(fixture.root)
	require.NoError(t, err)
	require.Equal(t, 1, digest.ProtoFiles)
	require.Equal(t, 1, digest.GeneratedFiles)
	require.NoError(t, writeFutuRepositoryDigest(fixture.root, digest))
	require.NoError(t, verifyFutuRepository(fixture.root, digest))

	require.NoError(t, os.WriteFile(pbPath, []byte("package tampered\n"), 0o600))
	tampered, err := inspectFutuRepository(fixture.root)
	require.NoError(t, err)
	err = verifyFutuRepository(fixture.root, tampered)
	require.ErrorContains(t, err, "checked-in OpenD generated outputs do not match")
}

func TestRepositoryDigestRejectsUnexpectedFilesAndProtoNames(t *testing.T) {
	fixture := newFutuFixture(t)
	protoDirectory := filepath.Join(fixture.root, "pkg", "futu", "proto")
	pbDirectory := filepath.Join(fixture.root, "pkg", "futu", "pb")

	require.NoError(t, os.WriteFile(filepath.Join(pbDirectory, "unexpected.txt"), []byte("bad"), 0o600))
	_, _, err := digestRepositoryTree(pbDirectory, ".go")
	require.ErrorContains(t, err, "unexpected file")

	require.NoError(t, os.Remove(filepath.Join(pbDirectory, "unexpected.txt")))
	require.NoError(t, os.Remove(filepath.Join(pbDirectory, "existing.pb.go")))
	require.NoError(t, os.WriteFile(filepath.Join(pbDirectory, "generated.go"), []byte("package generated\n"), 0o600))
	require.NoError(t, os.Remove(filepath.Join(protoDirectory, "existing.proto")))
	require.NoError(t, os.WriteFile(filepath.Join(protoDirectory, "Other.proto"), []byte("syntax = \"proto3\";\n"), 0o600))
	_, err = inspectFutuRepository(fixture.root)
	require.ErrorContains(t, err, "filenames do not match")
}

func TestParseRepositoryDigestValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), generatedDigestFilename)
	require.NoError(t, os.WriteFile(path, []byte(
		"proto "+string(make([]byte, 64))+" 1\n",
	), 0o600))
	_, err := parseFutuRepositoryDigest(path)
	assert.Error(t, err)
}
