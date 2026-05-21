#!/usr/bin/env bash
# Generate Go protobuf code from local Futu proto files.
#
# Source: ~/Downloads/FTAPIProtoFiles_10.5.6508 (Futu OpenAPI 10.5.6508)
# Target: pkg/futu/pb/<name>
#
# Requirements: protoc, protoc-gen-go
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC_DIR="${FUTU_PROTO_SRC:-${HOME}/Downloads/FTAPIProtoFiles_10.5.6508}"
EXTRA_PROTO_DIR="${REPO_ROOT}/scripts/futu-proto-overlays"
STAGE_DIR="${REPO_ROOT}/pkg/futu/proto"
OUT_DIR="${REPO_ROOT}/pkg/futu/pb"

if [ ! -d "${SRC_DIR}" ]; then
  echo "Futu proto source not found: ${SRC_DIR}" >&2
  exit 1
fi

PROTO_FILES=(
  Common.proto
  GetGlobalState.proto
  InitConnect.proto
  KeepAlive.proto
  Qot_Common.proto
  Qot_Sub.proto
  Qot_GetBasicQot.proto
  Qot_UpdateBasicQot.proto
  Qot_GetKL.proto
  Qot_RequestHistoryKL.proto
  Qot_GetStaticInfo.proto
  Trd_Common.proto
  Trd_GetAccList.proto
  Trd_GetFunds.proto
  Trd_GetPositionList.proto
  Trd_GetOrderList.proto
  Trd_GetHistoryOrderList.proto
  Trd_GetHistoryOrderFillList.proto
  Trd_GetOrderFee.proto
  Trd_PlaceOrder.proto
  Trd_ModifyOrder.proto
  Trd_Notify.proto
  Trd_UpdateOrder.proto
  Trd_UpdateOrderFill.proto
  Trd_UnlockTrade.proto
  Trd_SubAccPush.proto
)

EXTRA_PROTO_FILES=(
  Notify.proto
)

ALL_PROTO_FILES=("${PROTO_FILES[@]}" "${EXTRA_PROTO_FILES[@]}")

rm -rf "${STAGE_DIR}" "${OUT_DIR}"
mkdir -p "${STAGE_DIR}" "${OUT_DIR}"

for f in "${PROTO_FILES[@]}"; do
  if [ ! -f "${SRC_DIR}/${f}" ]; then
    echo "missing proto file: ${SRC_DIR}/${f}" >&2
    exit 1
  fi
  cp "${SRC_DIR}/${f}" "${STAGE_DIR}/${f}"
done

for f in "${EXTRA_PROTO_FILES[@]}"; do
  if [ ! -f "${EXTRA_PROTO_DIR}/${f}" ]; then
    echo "missing extra proto file: ${EXTRA_PROTO_DIR}/${f}" >&2
    exit 1
  fi
  cp "${EXTRA_PROTO_DIR}/${f}" "${STAGE_DIR}/${f}"
done

# Rewrite go_package to live under our module.
python3 - "${STAGE_DIR}" <<'PY'
import re, sys
from pathlib import Path

stage = Path(sys.argv[1])
for p in sorted(stage.glob("*.proto")):
    text = p.read_text()
    # find package
    m = re.search(r"^\s*package\s+([A-Za-z0-9_]+)\s*;", text, re.MULTILINE)
    if not m:
        raise SystemExit(f"no package in {p}")
    pkg = m.group(1)
    go_pkg_name = pkg.replace("_", "").lower()
    target = f'github.com/jftrade/jftrade-main/pkg/futu/pb/{go_pkg_name};{go_pkg_name}'
    if re.search(r"^\s*option\s+go_package\s*=", text, re.MULTILINE):
        text = re.sub(
            r"^\s*option\s+go_package\s*=.*?;",
            f'option go_package = "{target}";',
            text,
            count=1,
            flags=re.MULTILINE,
        )
    else:
        text = re.sub(
            r"(^\s*package\s+[A-Za-z0-9_]+\s*;)",
            r"\1\noption go_package = \"" + target + r"\";",
            text,
            count=1,
            flags=re.MULTILINE,
        )
    p.write_text(text)
print("[gen-futu-proto] rewrote go_package in", len(list(stage.glob("*.proto"))), "files")
PY

if ! command -v protoc >/dev/null 2>&1; then
  echo "protoc not found. brew install protobuf" >&2
  exit 1
fi
export PATH="$(go env GOPATH)/bin:${PATH}"
if ! command -v protoc-gen-go >/dev/null 2>&1; then
  echo "installing protoc-gen-go..." >&2
  GOFLAGS= go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
fi

protoc \
  --proto_path="${STAGE_DIR}" \
  --go_out="${OUT_DIR}" \
  --go_opt=paths=source_relative \
  $(printf '%s\n' "${ALL_PROTO_FILES[@]}" | sed "s#^#${STAGE_DIR}/#")

# Reorganize: protoc generates files in OUT_DIR root with original basenames.
# Move each <Name>.pb.go into pkg/futu/pb/<gopkg>/.
python3 - "${OUT_DIR}" <<'PY'
import re, sys, shutil
from pathlib import Path

out = Path(sys.argv[1])
for src in list(out.glob("*.pb.go")):
    text = src.read_text()
    m = re.search(r"^package\s+([A-Za-z0-9_]+)", text, re.MULTILINE)
    if not m:
        continue
    pkg = m.group(1)
    dest = out / pkg
    dest.mkdir(parents=True, exist_ok=True)
    shutil.move(str(src), dest / src.name)
print("[gen-futu-proto] reorganized files into per-package directories")
PY

echo "Done. Generated under ${OUT_DIR}"
