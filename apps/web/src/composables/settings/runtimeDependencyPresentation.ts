import type {
  RuntimeDependenciesResponseDto,
  RuntimeDependencyItemDto,
} from "@/contracts";
import type {
  RuntimeDependenciesResponse,
  RuntimeDependencyItem,
} from "@/types";

function mapRuntimeDependency(
  value: RuntimeDependencyItemDto,
): RuntimeDependencyItem {
  return {
    id: value.id ?? "",
    displayName: value.displayName ?? value.id ?? "",
    required: value.required ?? false,
    configurable: value.configurable ?? false,
    status: value.status ?? "error",
    minimumVersion: value.minimumVersion ?? "",
    detectedVersion: value.detectedVersion ?? "",
    configuredPath: value.configuredPath ?? "",
    effectivePath: value.effectivePath ?? "",
    resolvedPath: value.resolvedPath ?? "",
    source: value.source ?? "",
    homepageUrl: value.homepageUrl ?? "",
    message: value.message ?? "",
  };
}

export function mapRuntimeDependencies(
  value: RuntimeDependenciesResponseDto,
): RuntimeDependenciesResponse {
  return {
    checkedAt: value.checkedAt ?? "",
    allRequiredSatisfied: value.allRequiredSatisfied ?? false,
    dependencies: (value.dependencies ?? []).map(mapRuntimeDependency),
  };
}

export function dependencyStatusLabel(status: string): string {
  switch (status.toLowerCase()) {
    case "ok":
      return "可用";
    case "missing":
      return "缺失";
    case "outdated":
      return "版本过低";
    default:
      return "异常";
  }
}

export function dependencyStatusClass(status: string): string {
  switch (status.toLowerCase()) {
    case "ok":
      return "status-ok";
    case "missing":
    case "outdated":
      return "status-warning";
    default:
      return "status-error";
  }
}

export function dependencyVersionLabel(value: string): string {
  return value.trim() === "" ? "-" : value;
}

export function dependencyPathLabel(value: string): string {
  return value.trim() === "" ? "自动检测" : value;
}

export function dependencySourceLabel(value: string): string {
  if (value === "settings") return "设置";
  if (value === "path") return "PATH";
  if (value === "bundled") return "应用内嵌";
  if (value === "external-helper") return "Frozen helper";
  if (value === "workspace-venv") return "项目虚拟环境";
  if (value.startsWith("env:")) return value.replace("env:", "环境变量 ");
  return value || "-";
}
