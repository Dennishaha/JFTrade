package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jftrade/jftrade-main/cmd/internal/protogen"
)

var pineworkerProtoFiles = []string{
	"proto/pineworker_common.proto",
	"proto/pineworker_types.proto",
	"proto/pineworker.proto",
}

type generatorConfig struct {
	repoRoot          string
	maxGeneratedLines int
	runner            protogen.Runner
	stdout            io.Writer
	stderr            io.Writer
}

func generatePineworkerProto(cfg generatorConfig) error {
	protoDirectory := filepath.Join(cfg.repoRoot, "pkg", "strategy", "pineworker")
	outputDirectory := filepath.Join(protoDirectory, "pineworkerpb")
	if err := verifyPineworkerInputs(protoDirectory); err != nil {
		return err
	}
	environment, err := protogen.PrepareToolchain(protogen.ToolchainConfig{
		Runner: cfg.runner, Directory: cfg.repoRoot, Environment: protogen.InheritedEnvironment(),
		Stdout: cfg.stdout, Stderr: cfg.stderr, RequireGRPC: true,
	})
	if err != nil {
		return err
	}
	workspace, err := os.MkdirTemp(protoDirectory, ".generate-pineworker-proto-*")
	if err != nil {
		return fmt.Errorf("create Pineworker proto workspace: %w", err)
	}
	defer func() { _ = os.RemoveAll(workspace) }()
	rawDirectory := filepath.Join(workspace, "raw")
	nextDirectory := filepath.Join(workspace, "next")
	for _, directory := range []string{rawDirectory, nextDirectory} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("create generated output directory %s: %w", directory, err)
		}
	}
	if err := runPineworkerProtoc(cfg, environment, protoDirectory, rawDirectory); err != nil {
		return err
	}
	if err := flattenGeneratedFiles(rawDirectory, nextDirectory); err != nil {
		return err
	}
	if err := enforceGeneratedLineLimit(nextDirectory, cfg.maxGeneratedLines); err != nil {
		return err
	}
	if err := protogen.ReplaceDirectories([]protogen.Replacement{{
		Source: nextDirectory, Target: outputDirectory,
	}}); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cfg.stdout, "Done. Generated Pine worker protobuf code under %s\n", outputDirectory); err != nil {
		return fmt.Errorf("write completion status: %w", err)
	}
	return nil
}

func verifyPineworkerInputs(protoDirectory string) error {
	for _, relativePath := range pineworkerProtoFiles {
		path := filepath.Join(protoDirectory, filepath.FromSlash(relativePath))
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			if err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("inspect Pineworker proto file %s: %w", path, err)
			}
			return fmt.Errorf("missing Pineworker proto file: %s", path)
		}
	}
	return nil
}

func runPineworkerProtoc(
	cfg generatorConfig,
	environment []string,
	protoDirectory string,
	rawDirectory string,
) error {
	args := []string{
		"--proto_path=" + protoDirectory,
		"--go_out=" + rawDirectory,
		"--go-grpc_out=" + rawDirectory,
		"--go_opt=paths=source_relative",
		"--go-grpc_opt=paths=source_relative",
	}
	args = append(args, pineworkerProtoFiles...)
	if err := cfg.runner.Run(protogen.Command{
		Name: "protoc", Args: args, Dir: cfg.repoRoot, Env: environment,
		Stdout: cfg.stdout, Stderr: cfg.stderr,
	}); err != nil {
		return fmt.Errorf("generate Pineworker protobuf code: %w", err)
	}
	return nil
}
