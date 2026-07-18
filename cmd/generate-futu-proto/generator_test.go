package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/cmd/internal/protogen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateFutuProtoReplacesOutputsAfterSuccess(t *testing.T) {
	fixture := newFutuFixture(t)
	var protocArgs []string
	runner := fixture.runner(func(command protogen.Command) error {
		protocArgs = append([]string(nil), command.Args...)
		output := argumentValue(command.Args, "--go_out=")
		require.NotEmpty(t, output)
		require.NoError(t, os.WriteFile(filepath.Join(output, "Common.pb.go"), []byte("package common\n"), 0o600))
		return nil
	})

	err := generateFutuProto(generatorConfig{
		repoRoot: fixture.root, sourceDirectory: fixture.source,
		runner: runner, stdout: io.Discard, stderr: io.Discard,
	})
	require.NoError(t, err)
	assert.Equal(t, "--proto_path=", protocArgs[0][:len("--proto_path=")])
	assert.Equal(t, "--go_opt=paths=source_relative", protocArgs[2])
	_, err = os.Stat(filepath.Join(fixture.root, "pkg", "futu", "pb", "common", "Common.pb.go"))
	require.NoError(t, err)
	registration := readTestFile(t, filepath.Join(fixture.root, "pkg", "futu", "pb", "registerall", "register_all.go"))
	assert.Contains(t, registration, `_ "github.com/jftrade/jftrade-main/pkg/futu/pb/common"`)
	staged, err := os.ReadFile(filepath.Join(fixture.root, "pkg", "futu", "proto", "Common.proto"))
	require.NoError(t, err)
	assert.Contains(t, string(staged), `option go_package = "github.com/jftrade/jftrade-main/pkg/futu/pb/common;common";`)
	assert.NoFileExists(t, filepath.Join(fixture.root, "pkg", "futu", "pb", "existing.pb.go"))
}

func TestGenerateFutuProtoPreservesOutputsOnProtocFailure(t *testing.T) {
	fixture := newFutuFixture(t)
	runner := fixture.runner(func(protogen.Command) error { return errors.New("synthetic protoc failure") })

	err := generateFutuProto(generatorConfig{
		repoRoot: fixture.root, sourceDirectory: fixture.source,
		runner: runner, stdout: io.Discard, stderr: io.Discard,
	})
	require.ErrorContains(t, err, "synthetic protoc failure")
	assert.Equal(t, "existing proto", readTestFile(t, filepath.Join(fixture.root, "pkg", "futu", "proto", "existing.proto")))
	assert.Equal(t, "existing output", readTestFile(t, filepath.Join(fixture.root, "pkg", "futu", "pb", "existing.pb.go")))
	assert.Empty(t, matchingDirectories(t, fixture.root, ".generate-futu-proto-*"))
}

func TestGenerateFutuProtoRejectsChecksumBeforeCommands(t *testing.T) {
	fixture := newFutuFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(fixture.source, "Common.proto"), []byte("tampered"), 0o600))
	called := false

	err := generateFutuProto(generatorConfig{
		repoRoot: fixture.root, sourceDirectory: fixture.source,
		runner: protogen.RunnerFunc(func(protogen.Command) error { called = true; return nil }),
		stdout: io.Discard, stderr: io.Discard,
	})
	require.ErrorContains(t, err, "futu proto checksum mismatch")
	assert.False(t, called)
}

type futuFixture struct {
	root   string
	source string
	goPath string
}

var fixtureProtoFiles = []string{"Common.proto"}

func newFutuFixture(t *testing.T) futuFixture {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "source")
	for _, directory := range []string{
		source,
		filepath.Join(root, "scripts"),
		filepath.Join(root, "pkg", "futu", "proto"),
		filepath.Join(root, "pkg", "futu", "pb"),
	} {
		require.NoError(t, os.MkdirAll(directory, 0o755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n"), 0o600))
	var manifest strings.Builder
	for _, filename := range fixtureProtoFiles {
		content := []byte("syntax = \"proto3\";\npackage Common;\n")
		require.NoError(t, os.WriteFile(filepath.Join(source, filename), content, 0o600))
		digest := sha256.Sum256(content)
		_, _ = fmt.Fprintf(&manifest, "%x  %s\n", digest, filename)
	}
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "scripts", "futu-proto-10.9.6908.sha256"), []byte(manifest.String()), 0o600,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "pkg", "futu", "proto", "existing.proto"), []byte("existing proto"), 0o600,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "pkg", "futu", "pb", "existing.pb.go"), []byte("existing output"), 0o600,
	))
	return futuFixture{root: root, source: source, goPath: filepath.Join(root, "gopath")}
}

func (fixture futuFixture) runner(generate func(protogen.Command) error) protogen.Runner {
	return protogen.RunnerFunc(func(command protogen.Command) error {
		name := portableName(command.Name)
		switch {
		case name == "protoc" && len(command.Args) == 1 && command.Args[0] == "--version":
			_, _ = io.WriteString(command.Stdout, "libprotoc 34.1\n")
			return nil
		case name == "go" && assert.ObjectsAreEqual([]string{"env", "GOPATH"}, command.Args):
			_, _ = io.WriteString(command.Stdout, fixture.goPath+"\n")
			return nil
		case name == "protoc-gen-go" && len(command.Args) == 1 && command.Args[0] == "--version":
			_, _ = io.WriteString(command.Stdout, "protoc-gen-go v1.36.11\n")
			return nil
		case name == "protoc":
			return generate(command)
		default:
			return fmt.Errorf("unexpected command: %s %v", command.Name, command.Args)
		}
	})
}

func portableName(command string) string {
	name := filepath.Base(command)
	return strings.TrimSuffix(name, filepath.Ext(name))
}

func argumentValue(arguments []string, prefix string) string {
	for _, argument := range arguments {
		if after, ok := strings.CutPrefix(argument, prefix); ok {
			return after
		}
	}
	return ""
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(content)
}

func matchingDirectories(t *testing.T, root, pattern string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, pattern))
	require.NoError(t, err)
	return matches
}
