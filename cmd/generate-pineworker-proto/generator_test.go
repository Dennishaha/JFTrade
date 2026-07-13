package main

import (
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

func TestGeneratePineworkerProtoReplacesOutputAfterSuccess(t *testing.T) {
	fixture := newPineworkerFixture(t)
	var protocArgs []string
	runner := fixture.runner(func(command protogen.Command) error {
		protocArgs = append([]string(nil), command.Args...)
		return writePineGenerated(command, map[string]string{
			"proto/pineworker.pb.go":      "package pineworkerpb\n",
			"proto/pineworker_grpc.pb.go": "package pineworkerpb\n",
		})
	})

	err := generatePineworkerProto(generatorConfig{
		repoRoot: fixture.root, maxGeneratedLines: 1200,
		runner: runner, stdout: io.Discard, stderr: io.Discard,
	})
	require.NoError(t, err)
	assert.Contains(t, protocArgs, "--go-grpc_opt=paths=source_relative")
	assert.Equal(t, pineworkerProtoFiles, protocArgs[len(protocArgs)-len(pineworkerProtoFiles):])
	assert.FileExists(t, filepath.Join(fixture.output, "pineworker.pb.go"))
	assert.FileExists(t, filepath.Join(fixture.output, "pineworker_grpc.pb.go"))
	assert.NoFileExists(t, filepath.Join(fixture.output, "existing.pb.go"))
}

func TestGeneratePineworkerProtoPropagatesExitAndPreservesOutput(t *testing.T) {
	fixture := newPineworkerFixture(t)
	runner := fixture.runner(func(protogen.Command) error { return fakeProcessError{code: 42} })

	err := generatePineworkerProto(generatorConfig{
		repoRoot: fixture.root, maxGeneratedLines: 1200,
		runner: runner, stdout: io.Discard, stderr: io.Discard,
	})
	require.Error(t, err)
	assert.Equal(t, 42, protogen.ExitCode(err))
	assert.Equal(t, "existing output", readPineFile(t, filepath.Join(fixture.output, "existing.pb.go")))
	assert.Empty(t, pineWorkspaceMatches(t, fixture))
}

func TestGeneratePineworkerProtoPreservesOutputOnLineLimit(t *testing.T) {
	fixture := newPineworkerFixture(t)
	runner := fixture.runner(func(command protogen.Command) error {
		return writePineGenerated(command, map[string]string{
			"proto/pineworker.pb.go": "first\nsecond\n",
		})
	})

	err := generatePineworkerProto(generatorConfig{
		repoRoot: fixture.root, maxGeneratedLines: 1,
		runner: runner, stdout: io.Discard, stderr: io.Discard,
	})
	require.ErrorContains(t, err, "generated file exceeds 1 lines")
	assert.Equal(t, "existing output", readPineFile(t, filepath.Join(fixture.output, "existing.pb.go")))
}

func TestGeneratePineworkerProtoPreservesOutputOnCollision(t *testing.T) {
	fixture := newPineworkerFixture(t)
	runner := fixture.runner(func(command protogen.Command) error {
		return writePineGenerated(command, map[string]string{
			"one/same.go": "package one\n",
			"two/same.go": "package two\n",
		})
	})

	err := generatePineworkerProto(generatorConfig{
		repoRoot: fixture.root, maxGeneratedLines: 1200,
		runner: runner, stdout: io.Discard, stderr: io.Discard,
	})
	require.ErrorContains(t, err, "generated filename collision")
	assert.Equal(t, "existing output", readPineFile(t, filepath.Join(fixture.output, "existing.pb.go")))
}

func TestGeneratePineworkerProtoChecksInputsBeforeCommands(t *testing.T) {
	fixture := newPineworkerFixture(t)
	require.NoError(t, os.Remove(filepath.Join(
		fixture.root, "pkg", "strategy", "pineworker", filepath.FromSlash(pineworkerProtoFiles[0]),
	)))
	called := false

	err := generatePineworkerProto(generatorConfig{
		repoRoot: fixture.root, maxGeneratedLines: 1200,
		runner: protogen.RunnerFunc(func(protogen.Command) error { called = true; return nil }),
		stdout: io.Discard, stderr: io.Discard,
	})
	require.ErrorContains(t, err, "missing Pineworker proto file")
	assert.False(t, called)
}

type pineworkerFixture struct {
	root   string
	output string
	goPath string
}

func newPineworkerFixture(t *testing.T) pineworkerFixture {
	t.Helper()
	root := t.TempDir()
	protoRoot := filepath.Join(root, "pkg", "strategy", "pineworker")
	output := filepath.Join(protoRoot, "pineworkerpb")
	require.NoError(t, os.MkdirAll(filepath.Join(protoRoot, "proto"), 0o755))
	require.NoError(t, os.MkdirAll(output, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n"), 0o600))
	for _, relativePath := range pineworkerProtoFiles {
		require.NoError(t, os.WriteFile(
			filepath.Join(protoRoot, filepath.FromSlash(relativePath)), []byte("syntax = \"proto3\";\n"), 0o600,
		))
	}
	require.NoError(t, os.WriteFile(filepath.Join(output, "existing.pb.go"), []byte("existing output"), 0o600))
	return pineworkerFixture{root: root, output: output, goPath: filepath.Join(root, "gopath")}
}

func (fixture pineworkerFixture) runner(generate func(protogen.Command) error) protogen.Runner {
	return protogen.RunnerFunc(func(command protogen.Command) error {
		name := portablePineCommandName(command.Name)
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
		case name == "protoc-gen-go-grpc" && len(command.Args) == 1 && command.Args[0] == "--version":
			_, _ = io.WriteString(command.Stdout, "protoc-gen-go-grpc 1.6.2\n")
			return nil
		case name == "protoc":
			return generate(command)
		default:
			return fmt.Errorf("unexpected command: %s %v", command.Name, command.Args)
		}
	})
}

func writePineGenerated(command protogen.Command, files map[string]string) error {
	output := pineArgumentValue(command.Args, "--go_out=")
	if output == "" {
		return errors.New("missing --go_out")
	}
	for relativePath, content := range files {
		path := filepath.Join(output, filepath.FromSlash(relativePath))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return err
		}
	}
	return nil
}

type fakeProcessError struct {
	code int
}

func (err fakeProcessError) Error() string {
	return "synthetic protoc failure"
}

func (err fakeProcessError) ExitCode() int {
	return err.code
}

func portablePineCommandName(command string) string {
	name := filepath.Base(command)
	return strings.TrimSuffix(name, filepath.Ext(name))
}

func pineArgumentValue(arguments []string, prefix string) string {
	for _, argument := range arguments {
		if strings.HasPrefix(argument, prefix) {
			return strings.TrimPrefix(argument, prefix)
		}
	}
	return ""
}

func readPineFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(content)
}

func pineWorkspaceMatches(t *testing.T, fixture pineworkerFixture) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(
		fixture.root, "pkg", "strategy", "pineworker", ".generate-pineworker-proto-*",
	))
	require.NoError(t, err)
	return matches
}
