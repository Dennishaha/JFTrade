package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jftrade/jftrade-main/cmd/internal/protogen"
)

var futuProtoFiles = []string{
	"Common.proto", "GetGlobalState.proto", "InitConnect.proto", "KeepAlive.proto",
	"Qot_Common.proto", "Qot_Sub.proto", "Qot_GetBasicQot.proto", "Qot_UpdateBasicQot.proto",
	"Qot_GetKL.proto", "Qot_RequestHistoryKL.proto", "Qot_GetStaticInfo.proto",
	"Qot_GetUserSecurity.proto", "Qot_GetUserSecurityGroup.proto", "Trd_Common.proto",
	"Trd_GetAccList.proto", "Trd_GetFunds.proto", "Trd_GetPositionList.proto",
	"Trd_GetMaxTrdQtys.proto", "Trd_GetOrderList.proto", "Trd_GetOrderFillList.proto",
	"Trd_GetHistoryOrderList.proto", "Trd_GetHistoryOrderFillList.proto",
	"Trd_GetMarginRatio.proto", "Trd_GetOrderFee.proto", "Trd_FlowSummary.proto",
	"Trd_PlaceOrder.proto", "Trd_ModifyOrder.proto", "Trd_Notify.proto",
	"Trd_UpdateOrder.proto", "Trd_UpdateOrderFill.proto", "Trd_UnlockTrade.proto",
	"Trd_SubAccPush.proto",
}

var futuOverlayFiles = []string{
	"Notify.proto", "Qot_GetOrderBook.proto", "Qot_GetSecuritySnapshot.proto", "Qot_UpdateOrderBook.proto",
}

type generatorConfig struct {
	repoRoot        string
	sourceDirectory string
	runner          protogen.Runner
	stdout          io.Writer
	stderr          io.Writer
}

func generateFutuProto(cfg generatorConfig) error {
	manifestPath := filepath.Join(cfg.repoRoot, "scripts", "futu-proto-10.5.6508.sha256")
	overlayDirectory := filepath.Join(cfg.repoRoot, "scripts", "futu-proto-overlays")
	if err := verifyFutuInputs(cfg.sourceDirectory, manifestPath, futuProtoFiles); err != nil {
		return err
	}
	if err := verifyOverlayInputs(overlayDirectory); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cfg.stdout, "[generate-futu-proto] verified Futu OpenAPI 10.5.6508 inputs (%d files)\n", len(futuProtoFiles)); err != nil {
		return fmt.Errorf("write verification status: %w", err)
	}
	environment, err := protogen.PrepareToolchain(protogen.ToolchainConfig{
		Runner: cfg.runner, Directory: cfg.repoRoot, Environment: protogen.InheritedEnvironment(),
		Stdout: cfg.stdout, Stderr: cfg.stderr,
	})
	if err != nil {
		return err
	}
	workspace, err := os.MkdirTemp(cfg.repoRoot, ".generate-futu-proto-*")
	if err != nil {
		return fmt.Errorf("create Futu proto workspace: %w", err)
	}
	defer func() { _ = os.RemoveAll(workspace) }()
	stageDirectory := filepath.Join(workspace, "proto")
	outputDirectory := filepath.Join(workspace, "pb")
	if err := os.MkdirAll(stageDirectory, 0o755); err != nil {
		return fmt.Errorf("create staged proto directory: %w", err)
	}
	if err := os.MkdirAll(outputDirectory, 0o755); err != nil {
		return fmt.Errorf("create generated output directory: %w", err)
	}
	if err := stageFutuInputs(cfg.sourceDirectory, overlayDirectory, stageDirectory); err != nil {
		return err
	}
	rewritten, err := rewriteGoPackages(stageDirectory)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cfg.stdout, "[generate-futu-proto] rewrote go_package in %d files\n", rewritten); err != nil {
		return fmt.Errorf("write rewrite status: %w", err)
	}
	if err := runFutuProtoc(cfg, environment, stageDirectory, outputDirectory); err != nil {
		return err
	}
	if _, err := organizeGeneratedFiles(outputDirectory); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(cfg.stdout, "[generate-futu-proto] reorganized files into per-package directories"); err != nil {
		return fmt.Errorf("write organization status: %w", err)
	}
	if err := protogen.ReplaceDirectories([]protogen.Replacement{
		{Source: stageDirectory, Target: filepath.Join(cfg.repoRoot, "pkg", "futu", "proto")},
		{Source: outputDirectory, Target: filepath.Join(cfg.repoRoot, "pkg", "futu", "pb")},
	}); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cfg.stdout, "Done. Generated under %s\n", filepath.Join(cfg.repoRoot, "pkg", "futu", "pb")); err != nil {
		return fmt.Errorf("write completion status: %w", err)
	}
	return nil
}

func verifyOverlayInputs(directory string) error {
	for _, filename := range futuOverlayFiles {
		path := filepath.Join(directory, filename)
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			if err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("inspect extra proto file %s: %w", path, err)
			}
			return fmt.Errorf("missing extra proto file: %s", path)
		}
	}
	return nil
}

func stageFutuInputs(sourceDirectory, overlayDirectory, stageDirectory string) error {
	for _, filename := range futuProtoFiles {
		if err := protogen.CopyFile(
			filepath.Join(sourceDirectory, filename), filepath.Join(stageDirectory, filename),
		); err != nil {
			return err
		}
	}
	for _, filename := range futuOverlayFiles {
		if err := protogen.CopyFile(
			filepath.Join(overlayDirectory, filename), filepath.Join(stageDirectory, filename),
		); err != nil {
			return err
		}
	}
	return nil
}

func runFutuProtoc(cfg generatorConfig, environment []string, stageDirectory, outputDirectory string) error {
	allFiles := append(append([]string(nil), futuProtoFiles...), futuOverlayFiles...)
	args := []string{
		"--proto_path=" + stageDirectory,
		"--go_out=" + outputDirectory,
		"--go_opt=paths=source_relative",
	}
	for _, filename := range allFiles {
		args = append(args, filepath.Join(stageDirectory, filename))
	}
	if err := cfg.runner.Run(protogen.Command{
		Name: "protoc", Args: args, Dir: cfg.repoRoot, Env: environment,
		Stdout: cfg.stdout, Stderr: cfg.stderr,
	}); err != nil {
		return fmt.Errorf("generate Futu protobuf code: %w", err)
	}
	return nil
}
