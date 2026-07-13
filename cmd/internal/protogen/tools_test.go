package protogen

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareToolchainInstallsMissingPlugins(t *testing.T) {
	goPath := t.TempDir()
	installed := make(map[string]bool)
	var commands []Command
	runner := RunnerFunc(func(command Command) error {
		commands = append(commands, command)
		name := portableCommandName(command.Name)
		switch {
		case name == "protoc" && equalArgs(command.Args, "--version"):
			_, _ = io.WriteString(command.Stdout, "libprotoc 34.1\n")
		case name == "go" && equalArgs(command.Args, "env", "GOPATH"):
			_, _ = io.WriteString(command.Stdout, goPath+"\n")
		case name == "go" && len(command.Args) == 2 && command.Args[0] == "install":
			if strings.Contains(command.Args[1], "protoc-gen-go-grpc") {
				installed["protoc-gen-go-grpc"] = true
			} else {
				installed["protoc-gen-go"] = true
			}
		case name == "protoc-gen-go":
			if !installed[name] {
				return errors.New("missing")
			}
			_, _ = io.WriteString(command.Stdout, "protoc-gen-go v1.36.11\n")
		case name == "protoc-gen-go-grpc":
			if !installed[name] {
				return errors.New("missing")
			}
			_, _ = io.WriteString(command.Stdout, "protoc-gen-go-grpc 1.6.2\n")
		default:
			return errors.New("unexpected command")
		}
		return nil
	})
	var stderr strings.Builder
	environment, err := PrepareToolchain(ToolchainConfig{
		Runner: runner, Environment: []string{"PATH=existing", "GOFLAGS=-tags=test"},
		Stdout: io.Discard, Stderr: &stderr, RequireGRPC: true,
	})
	require.NoError(t, err)
	assert.True(t, installed["protoc-gen-go"])
	assert.True(t, installed["protoc-gen-go-grpc"])
	assert.Contains(t, stderr.String(), "installing google.golang.org/protobuf")
	assert.Equal(t, filepath.Join(goPath, "bin")+string(os.PathListSeparator)+"existing", environmentValue(environment, "PATH"))
	for _, command := range commands {
		if portableCommandName(command.Name) == "go" && len(command.Args) > 0 && command.Args[0] == "install" {
			assert.Equal(t, "", environmentValue(command.Env, "GOFLAGS"))
		}
	}
}

func TestPrepareToolchainRejectsWrongProtoc(t *testing.T) {
	runner := RunnerFunc(func(command Command) error {
		_, _ = io.WriteString(command.Stdout, "libprotoc 33.0\n")
		return nil
	})
	_, err := PrepareToolchain(ToolchainConfig{Runner: runner, Stderr: io.Discard})
	require.EqualError(t, err, "protoc 34.1 required, found: libprotoc 33.0")
}

func TestEnvironmentHelpersAreCaseInsensitive(t *testing.T) {
	environment := setEnvironment([]string{"Path=one", "OTHER=two"}, "PATH", "three")
	assert.Equal(t, "three", environmentValue(environment, "path"))
	assert.Equal(t, "two", environmentValue(environment, "other"))
	assert.Len(t, environment, 2)
}

func portableCommandName(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if name == "protoc-gen-go-grpc" || name == "protoc-gen-go" || name == "protoc" || name == "go" {
		return name
	}
	return filepath.Base(path)
}

func equalArgs(actual []string, expected ...string) bool {
	return assert.ObjectsAreEqual(expected, actual)
}
