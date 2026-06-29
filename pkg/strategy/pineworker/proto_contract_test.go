package pineworker

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestPineWorkerProtoCompilesAndExposesContract(t *testing.T) {
	protoc, err := exec.LookPath("protoc")
	if err != nil {
		t.Skip("protoc not installed")
	}
	tmpDir := t.TempDir()
	descriptorPath := filepath.Join(tmpDir, "pineworker.pb")
	protoPath := filepath.Join("proto", "pineworker.proto")
	cmd := exec.Command(protoc,
		"--proto_path=.",
		"--descriptor_set_out="+descriptorPath,
		"--include_imports",
		protoPath,
	)
	cmd.Dir = "."
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("protoc %s failed: %v\n%s", protoPath, err, string(output))
	}
	raw, err := os.ReadFile(descriptorPath)
	if err != nil {
		t.Fatalf("read descriptor: %v", err)
	}
	var files descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(raw, &files); err != nil {
		t.Fatalf("unmarshal descriptor: %v", err)
	}
	file := findProtoFile(t, &files, "proto/pineworker.proto")
	typesFile := findProtoFile(t, &files, "proto/pineworker_types.proto")
	if file.GetPackage() != "jftrade.strategy.pineworker.v1" {
		t.Fatalf("package = %q", file.GetPackage())
	}
	if !fileImports(file, "proto/pineworker_types.proto") {
		t.Fatal("pineworker.proto must import pineworker_types.proto")
	}
	if !fileImports(typesFile, "proto/pineworker_common.proto") {
		t.Fatal("pineworker_types.proto must import pineworker_common.proto")
	}
	service := findService(t, file, "PineWorker")
	for _, method := range []string{"HealthCheck", "AnalyzeScript", "RunScript"} {
		if !serviceHasMethod(service, method) {
			t.Fatalf("service PineWorker missing method %s", method)
		}
	}
	runRequest := findMessage(t, typesFile, "RunScriptRequest")
	for _, field := range []string{"job_id", "script_id", "source", "symbol", "timeframe", "mode", "candles", "params"} {
		if !messageHasField(runRequest, field) {
			t.Fatalf("RunScriptRequest missing field %s", field)
		}
	}
	commonFile := findProtoFile(t, &files, "proto/pineworker_common.proto")
	intent := findMessage(t, commonFile, "OrderIntent")
	for _, field := range []string{"kind", "id", "direction", "quantity", "quantity_pct", "limit_price", "stop_price", "has_quantity", "has_limit_price"} {
		if !messageHasField(intent, field) {
			t.Fatalf("OrderIntent missing field %s", field)
		}
	}
}

func fileImports(file *descriptorpb.FileDescriptorProto, name string) bool {
	return slices.Contains(file.GetDependency(), name)
}

func findProtoFile(t *testing.T, files *descriptorpb.FileDescriptorSet, name string) *descriptorpb.FileDescriptorProto {
	t.Helper()
	for _, file := range files.GetFile() {
		if file.GetName() == name {
			return file
		}
	}
	t.Fatalf("descriptor missing file %s", name)
	return nil
}

func findService(t *testing.T, file *descriptorpb.FileDescriptorProto, name string) *descriptorpb.ServiceDescriptorProto {
	t.Helper()
	for _, service := range file.GetService() {
		if service.GetName() == name {
			return service
		}
	}
	t.Fatalf("descriptor missing service %s", name)
	return nil
}

func serviceHasMethod(service *descriptorpb.ServiceDescriptorProto, name string) bool {
	for _, method := range service.GetMethod() {
		if method.GetName() == name {
			return true
		}
	}
	return false
}

func findMessage(t *testing.T, file *descriptorpb.FileDescriptorProto, name string) *descriptorpb.DescriptorProto {
	t.Helper()
	for _, message := range file.GetMessageType() {
		if message.GetName() == name {
			return message
		}
	}
	t.Fatalf("descriptor missing message %s", name)
	return nil
}

func messageHasField(message *descriptorpb.DescriptorProto, name string) bool {
	for _, field := range message.GetField() {
		if field.GetName() == name {
			return true
		}
	}
	return false
}
