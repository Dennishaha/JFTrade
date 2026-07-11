#!/usr/bin/env bash
# Generate Go protobuf and gRPC code for the PineTS worker boundary.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROTO_DIR="${REPO_ROOT}/pkg/strategy/pineworker"
OUT_DIR="${REPO_ROOT}/pkg/strategy/pineworker/pineworkerpb"
MAX_LINES="${MAX_GENERATED_LINES:-1200}"
TMP_DIR="$(mktemp -d "${OUT_DIR}.tmp.XXXXXX")"
RAW_OUT="${TMP_DIR}/raw"
NEXT_OUT="${TMP_DIR}/next"
cleanup() {
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

if ! command -v protoc >/dev/null 2>&1; then
  echo "protoc not found. brew install protobuf" >&2
  exit 1
fi

export PATH="$(go env GOPATH)/bin:${PATH}"
if ! command -v protoc-gen-go >/dev/null 2>&1; then
  echo "installing protoc-gen-go..." >&2
  GOFLAGS= go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
fi
if ! command -v protoc-gen-go-grpc >/dev/null 2>&1; then
  echo "installing protoc-gen-go-grpc..." >&2
  GOFLAGS= go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
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
