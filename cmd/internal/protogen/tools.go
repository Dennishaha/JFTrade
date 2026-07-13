package protogen

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	ProtocVersion         = "34.1"
	ProtocGenGoVersion    = "v1.36.11"
	ProtocGenGRPCVersion  = "1.6.2"
	protocGenGoModule     = "google.golang.org/protobuf/cmd/protoc-gen-go@" + ProtocGenGoVersion
	protocGenGRPCGoModule = "google.golang.org/grpc/cmd/protoc-gen-go-grpc@v" + ProtocGenGRPCVersion
)

// ToolchainConfig controls protobuf tool discovery and installation.
type ToolchainConfig struct {
	Runner      Runner
	Directory   string
	Environment []string
	Stdout      io.Writer
	Stderr      io.Writer
	RequireGRPC bool
}

// PrepareToolchain validates protoc, installs missing Go plugins, and returns
// an environment in which protoc can discover those plugins.
func PrepareToolchain(cfg ToolchainConfig) ([]string, error) {
	if err := requireProtoc(cfg); err != nil {
		return nil, err
	}
	goBin, err := goBinDirectory(cfg)
	if err != nil {
		return nil, err
	}
	environment := prependPath(cfg.Environment, goBin)
	plugins := []pluginRequirement{{
		name: "protoc-gen-go", expected: "protoc-gen-go " + ProtocGenGoVersion,
		module: protocGenGoModule,
	}}
	if cfg.RequireGRPC {
		plugins = append(plugins, pluginRequirement{
			name: "protoc-gen-go-grpc", expected: "protoc-gen-go-grpc " + ProtocGenGRPCVersion,
			module: protocGenGRPCGoModule,
		})
	}
	for _, plugin := range plugins {
		if err := ensurePlugin(cfg, environment, goBin, plugin); err != nil {
			return nil, err
		}
	}
	return environment, nil
}

type pluginRequirement struct {
	name     string
	expected string
	module   string
}

func requireProtoc(cfg ToolchainConfig) error {
	actual, err := commandVersion(cfg, cfg.Environment, "protoc")
	if err != nil {
		return fmt.Errorf("protoc not found or unusable; install protoc %s: %w", ProtocVersion, err)
	}
	expected := "libprotoc " + ProtocVersion
	if actual != expected {
		return fmt.Errorf("protoc %s required, found: %s", ProtocVersion, displayVersion(actual))
	}
	return nil
}

func goBinDirectory(cfg ToolchainConfig) (string, error) {
	var output bytes.Buffer
	err := cfg.Runner.Run(Command{
		Name: "go", Args: []string{"env", "GOPATH"}, Dir: cfg.Directory,
		Env: cfg.Environment, Stdout: &output, Stderr: cfg.Stderr,
	})
	if err != nil {
		return "", fmt.Errorf("resolve GOPATH: %w", err)
	}
	goPaths := filepath.SplitList(strings.TrimSpace(output.String()))
	if len(goPaths) == 0 || goPaths[0] == "" {
		return "", fmt.Errorf("resolve GOPATH: go env GOPATH returned an empty path")
	}
	return filepath.Join(goPaths[0], "bin"), nil
}

func ensurePlugin(
	cfg ToolchainConfig,
	environment []string,
	goBin string,
	plugin pluginRequirement,
) error {
	commandName := installedPluginPath(goBin, plugin.name)
	actual, versionErr := commandVersion(cfg, environment, commandName)
	if versionErr == nil && actual == plugin.expected {
		return nil
	}
	if _, err := fmt.Fprintf(cfg.Stderr, "installing %s...\n", plugin.module); err != nil {
		return fmt.Errorf("write plugin installation status: %w", err)
	}
	installEnvironment := setEnvironment(environment, "GOFLAGS", "")
	if err := cfg.Runner.Run(Command{
		Name: "go", Args: []string{"install", plugin.module}, Dir: cfg.Directory,
		Env: installEnvironment, Stdout: cfg.Stdout, Stderr: cfg.Stderr,
	}); err != nil {
		return fmt.Errorf("install %s: %w", plugin.name, err)
	}
	commandName = installedPluginPath(goBin, plugin.name)
	actual, versionErr = commandVersion(cfg, environment, commandName)
	if versionErr != nil {
		return fmt.Errorf("%s %s required, found: missing: %w", plugin.name, expectedPluginVersion(plugin), versionErr)
	}
	if actual != plugin.expected {
		return fmt.Errorf("%s %s required, found: %s", plugin.name, expectedPluginVersion(plugin), displayVersion(actual))
	}
	return nil
}

func commandVersion(cfg ToolchainConfig, environment []string, name string) (string, error) {
	var output bytes.Buffer
	err := cfg.Runner.Run(Command{
		Name: name, Args: []string{"--version"}, Dir: cfg.Directory,
		Env: environment, Stdout: &output, Stderr: io.Discard,
	})
	return strings.TrimSpace(output.String()), err
}

func installedPluginPath(goBin, name string) string {
	candidate := filepath.Join(goBin, executableName(name))
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		return candidate
	}
	return name
}

func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func expectedPluginVersion(plugin pluginRequirement) string {
	return strings.TrimPrefix(plugin.expected, plugin.name+" ")
}

func displayVersion(version string) string {
	if version == "" {
		return "missing"
	}
	return version
}

func prependPath(environment []string, directory string) []string {
	current := environmentValue(environment, "PATH")
	if current != "" {
		directory += string(os.PathListSeparator) + current
	}
	return setEnvironment(environment, "PATH", directory)
}

func environmentValue(environment []string, name string) string {
	for _, entry := range environment {
		key, value, found := strings.Cut(entry, "=")
		if found && strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

func setEnvironment(environment []string, name, value string) []string {
	result := make([]string, 0, len(environment)+1)
	found := false
	for _, entry := range environment {
		key, _, valid := strings.Cut(entry, "=")
		if valid && strings.EqualFold(key, name) {
			if !found {
				result = append(result, name+"="+value)
				found = true
			}
			continue
		}
		result = append(result, entry)
	}
	if !found {
		result = append(result, name+"="+value)
	}
	return result
}
