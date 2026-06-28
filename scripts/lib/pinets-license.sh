#!/usr/bin/env bash

pinets_license_blocked=0

pinets_mark_blocked() {
  echo "BLOCKED: $*" >&2
  pinets_license_blocked=1
}

pinets_check_package_and_license() {
  pinets_license_blocked=0

  local pinets_check_status="${JFTRADE_PINETS_RELEASE_PINETS_STATUS:-}"
  local pinets_license="${JFTRADE_PINETS_RELEASE_PINETS_LICENSE:-}"
  local commercial_license_ack="${JFTRADE_PINETS_COMMERCIAL_LICENSE_ACK:-}"

  echo "==> Checking commercial pinets package"
  if [[ -n "$pinets_check_status" ]]; then
    if [[ "$pinets_check_status" != "0" ]]; then
      pinets_mark_blocked "pinets package is not installed or not visible to npm workspaces"
    fi
  elif ! npm ls pinets --workspaces --depth=1; then
    pinets_mark_blocked "pinets package is not installed or not visible to npm workspaces"
  fi

  if [[ -z "$pinets_license" && "$pinets_license_blocked" -eq 0 ]]; then
    pinets_license="$(node -e "const pkg=require('./node_modules/pinets/package.json'); console.log(pkg.license || '')" 2>/dev/null || true)"
  fi
  if [[ "$pinets_license_blocked" -eq 0 ]]; then
    echo "==> Checking PineTS commercial license attestation"
    if [[ "$commercial_license_ack" != "1" ]]; then
      pinets_mark_blocked "commercial PineTS license attestation is missing; set JFTRADE_PINETS_COMMERCIAL_LICENSE_ACK=1 only after legal approval"
    fi
    case "$pinets_license" in
      ""|"UNLICENSED"|"Commercial"|"commercial"|"SEE LICENSE IN LICENSE"|"SEE LICENSE IN LICENSE.md")
        ;;
      *)
        pinets_mark_blocked "pinets package license is ${pinets_license}; release requires a recorded commercial license approval"
        ;;
    esac
  fi

  [[ "$pinets_license_blocked" -eq 0 ]]
}
