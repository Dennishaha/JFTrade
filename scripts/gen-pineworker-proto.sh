#!/usr/bin/env bash
# Generate Go protobuf and gRPC code for the PineTS worker boundary.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROTO_DIR="${REPO_ROOT}/pkg/strategy/pineworker"
OUT_DIR="${REPO_ROOT}/pkg/strategy/pineworker/pineworkerpb"
MAX_LINES="${MAX_GENERATED_LINES:-1200}"
PROTOC_VERSION="34.1"
PROTOC_GEN_GO_VERSION="v1.36.11"
PROTOC_GEN_GO_GRPC_VERSION="1.6.2"
TMP_DIR="$(mktemp -d "${OUT_DIR}.tmp.XXXXXX")"
RAW_OUT="${TMP_DIR}/raw"
NEXT_OUT="${TMP_DIR}/next"
cleanup() {
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

if ! command -v protoc >/dev/null 2>&1; then
  echo "protoc not found. Install protoc ${PROTOC_VERSION}." >&2
  exit 1
fi
actual_protoc_version="$(protoc --version)"
if [ "${actual_protoc_version}" != "libprotoc ${PROTOC_VERSION}" ]; then
  echo "protoc ${PROTOC_VERSION} required, found: ${actual_protoc_version}" >&2
  exit 1
fi

export PATH="$(go env GOPATH)/bin:${PATH}"
actual_protoc_gen_go_version="$(protoc-gen-go --version 2>/dev/null || true)"
if [ "${actual_protoc_gen_go_version}" != "protoc-gen-go ${PROTOC_GEN_GO_VERSION}" ]; then
  echo "installing protoc-gen-go ${PROTOC_GEN_GO_VERSION}..." >&2
  GOFLAGS= go install "google.golang.org/protobuf/cmd/protoc-gen-go@${PROTOC_GEN_GO_VERSION}"
  actual_protoc_gen_go_version="$(protoc-gen-go --version 2>/dev/null || true)"
  if [ "${actual_protoc_gen_go_version}" != "protoc-gen-go ${PROTOC_GEN_GO_VERSION}" ]; then
    echo "protoc-gen-go ${PROTOC_GEN_GO_VERSION} required, found: ${actual_protoc_gen_go_version:-missing}" >&2
    exit 1
  fi
fi
actual_protoc_gen_go_grpc_version="$(protoc-gen-go-grpc --version 2>/dev/null || true)"
if [ "${actual_protoc_gen_go_grpc_version}" != "protoc-gen-go-grpc ${PROTOC_GEN_GO_GRPC_VERSION}" ]; then
  echo "installing protoc-gen-go-grpc ${PROTOC_GEN_GO_GRPC_VERSION}..." >&2
  GOFLAGS= go install "google.golang.org/grpc/cmd/protoc-gen-go-grpc@v${PROTOC_GEN_GO_GRPC_VERSION}"
  actual_protoc_gen_go_grpc_version="$(protoc-gen-go-grpc --version 2>/dev/null || true)"
  if [ "${actual_protoc_gen_go_grpc_version}" != "protoc-gen-go-grpc ${PROTOC_GEN_GO_GRPC_VERSION}" ]; then
    echo "protoc-gen-go-grpc ${PROTOC_GEN_GO_GRPC_VERSION} required, found: ${actual_protoc_gen_go_grpc_version:-missing}" >&2
    exit 1
  fi
fi

mkdir -p "${RAW_OUT}" "${NEXT_OUT}"

protoc \
  --proto_path="${PROTO_DIR}" \
  --go_out="${RAW_OUT}" \
  --go-grpc_out="${RAW_OUT}" \
  --go_opt=paths=source_relative \
  --go-grpc_opt=paths=source_relative \
  proto/pineworker_common.proto \
  proto/pineworker_types.proto \
  proto/pineworker.proto

find "${RAW_OUT}" -maxdepth 3 -name '*.go' -print0 | while IFS= read -r -d '' file; do
  mv "${file}" "${NEXT_OUT}/$(basename "${file}")"
done

find "${NEXT_OUT}" -name '*.go' -print0 | while IFS= read -r -d '' file; do
  lines="$(wc -l <"${file}")"
  if [ "${lines}" -gt "${MAX_LINES}" ]; then
    echo "generated file exceeds ${MAX_LINES} lines: ${file} (${lines})" >&2
    exit 1
  fi
done

if [ -e "${OUT_DIR}" ]; then
  mv "${OUT_DIR}" "${TMP_DIR}/previous"
fi
if ! mv "${NEXT_OUT}" "${OUT_DIR}"; then
  if [ -e "${TMP_DIR}/previous" ]; then
    mv "${TMP_DIR}/previous" "${OUT_DIR}"
  fi
  exit 1
fi

echo "Done. Generated Pine worker protobuf code under ${OUT_DIR}"
