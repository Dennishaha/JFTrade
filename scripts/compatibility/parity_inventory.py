#!/usr/bin/env python3
"""Shared loading and validation for the versioned Go→Rust parity inventory."""

from __future__ import annotations

import datetime as _datetime
import json
import re
from pathlib import Path
from typing import Any


SCHEMA_VERSION = "jftrade.go-rust-parity-mappings.v2"
_SHA256 = re.compile(r"^[0-9a-f]{64}$")
_SHA1 = re.compile(r"^[0-9a-f]{40}$")


def load_inventory(path: str | Path) -> tuple[dict[str, dict[str, Any]], dict[str, Any]]:
    """Load a v2 envelope and return ``(mappings, document)``.

    The legacy bare mapping shape remains readable for historical fixtures and
    unit tests. Repository documents must use the v2 envelope; callers can use
    :func:`validate_document` to enforce that policy.
    """

    document = json.loads(Path(path).read_text(encoding="utf-8"))
    if isinstance(document, dict) and "mappings" in document:
        mappings = document["mappings"]
        if not isinstance(mappings, dict):
            raise ValueError("parity inventory mappings must be an object")
        return mappings, document
    if not isinstance(document, dict):
        raise ValueError("parity inventory must be a JSON object")
    return document, {"schemaVersion": "legacy", "mappings": document}


def validate_document(document: dict[str, Any], *, require_v2: bool = True) -> list[str]:
    """Return machine-readable envelope and metadata validation errors."""

    errors: list[str] = []
    if require_v2 and document.get("schemaVersion") != SCHEMA_VERSION:
        errors.append(f"schemaVersion must be {SCHEMA_VERSION}")
    if "mappings" not in document:
        if require_v2:
            errors.append("mappings is required")
        return errors
    mappings = document.get("mappings")
    if not isinstance(mappings, dict):
        errors.append("mappings must be an object")
        return errors
    reuse = document.get("reuse")
    if not isinstance(reuse, dict):
        errors.append("reuse must be an object")

    baseline = document.get("baseline")
    if not isinstance(baseline, dict):
        errors.append("baseline must be an object")
    else:
        for field in ("commit", "treeSha", "sourceRef"):
            if not isinstance(baseline.get(field), str) or not baseline[field]:
                errors.append(f"baseline.{field} is required")
        if isinstance(baseline.get("commit"), str) and not _SHA1.fullmatch(baseline["commit"]):
            errors.append("baseline.commit must be a 40-character lowercase SHA")
        if isinstance(baseline.get("treeSha"), str) and not _SHA1.fullmatch(baseline["treeSha"]):
            errors.append("baseline.treeSha must be a 40-character lowercase SHA")

    generator = document.get("generatorVersion")
    if not isinstance(generator, str) or not generator:
        errors.append("generatorVersion is required")
    generated_at = document.get("generatedAt")
    if not isinstance(generated_at, str) or not generated_at:
        errors.append("generatedAt is required")
    else:
        try:
            _datetime.datetime.fromisoformat(generated_at.replace("Z", "+00:00"))
        except ValueError:
            errors.append("generatedAt must be an ISO-8601 timestamp")

    expected_reuse: dict[str, list[str]] = {}
    for key, item in mappings.items():
        if not isinstance(item, dict):
            errors.append(f"{key}: mapping must be an object")
            continue
        source = item.get("source")
        if not isinstance(source, dict):
            errors.append(f"{key}: source must be an object")
        else:
            for field in ("file", "line", "test", "blobSha"):
                if field not in source:
                    errors.append(f"{key}: source.{field} is required")
            if not isinstance(source.get("line"), int) or source.get("line", 0) <= 0:
                errors.append(f"{key}: source.line must be a positive integer")
            if isinstance(source.get("blobSha"), str) and not _SHA1.fullmatch(source["blobSha"]):
                errors.append(f"{key}: source.blobSha must be a 40-character lowercase SHA")

        evidence = item.get("rustEvidence")
        if not isinstance(evidence, list):
            errors.append(f"{key}: rustEvidence must be an array")
        else:
            for index, target in enumerate(evidence):
                if not isinstance(target, dict):
                    errors.append(f"{key}: rustEvidence[{index}] must be an object")
                    continue
                for field in ("file", "test", "crate"):
                    if not isinstance(target.get(field), str) or not target[field]:
                        errors.append(f"{key}: rustEvidence[{index}].{field} is required")
                if all(isinstance(target.get(field), str) and target[field] for field in ("file", "test")):
                    target_id = f"{target['file']}::{target['test']}"
                    expected_reuse.setdefault(target_id, []).append(key)

        for field in ("coveredAssertions", "uncoveredAssertions"):
            if not isinstance(item.get(field), list) or not all(
                isinstance(value, str) for value in item[field]
            ):
                errors.append(f"{key}: {field} must be an array of strings")
        if "boundaryReason" not in item:
            errors.append(f"{key}: boundaryReason is required")
        elif item["boundaryReason"] is not None and not isinstance(item["boundaryReason"], str):
            errors.append(f"{key}: boundaryReason must be a string or null")

        coverage = item.get("assertionCoverage")
        if not isinstance(coverage, dict):
            errors.append(f"{key}: assertionCoverage must be an object")
        else:
            for field in ("covered", "uncovered"):
                if not isinstance(coverage.get(field), list):
                    errors.append(f"{key}: assertionCoverage.{field} must be an array")

        verification = item.get("verification")
        if not isinstance(verification, dict):
            errors.append(f"{key}: verification must be an object")
        else:
            if not isinstance(verification.get("argv"), list) or not all(
                isinstance(value, str) for value in verification["argv"]
            ):
                errors.append(f"{key}: verification.argv must be an array of strings")
            if verification.get("status") not in {"passed", "failed", "unverified"}:
                errors.append(f"{key}: verification.status is invalid")
            if not isinstance(verification.get("testFilter"), list) or not all(
                isinstance(value, str) for value in verification["testFilter"]
            ):
                errors.append(f"{key}: verification.testFilter must be an array of strings")
            for field in ("runnerVersion", "toolchain", "exitCode", "verifiedCommit", "verifiedAt", "receiptDigest"):
                if field not in verification:
                    errors.append(f"{key}: verification.{field} is required")

        for field in ("risk", "owner", "priority", "nextAction"):
            if not isinstance(item.get(field), str) or not item[field]:
                errors.append(f"{key}: {field} is required")
        if not isinstance(item.get("blockedBy"), list):
            errors.append(f"{key}: blockedBy must be an array")

    if isinstance(reuse, dict):
        for target_id, entry in reuse.items():
            if not isinstance(entry, dict):
                errors.append(f"reuse.{target_id} must be an object")
                continue
            references = entry.get("referenceKeys")
            if not isinstance(references, list) or not all(isinstance(value, str) for value in references):
                errors.append(f"reuse.{target_id}.referenceKeys must be an array of strings")
                continue
            if not isinstance(entry.get("referenceCount"), int) or entry["referenceCount"] != len(references):
                errors.append(f"reuse.{target_id}.referenceCount must equal referenceKeys length")
            if len(set(references)) != len(references):
                errors.append(f"reuse.{target_id}.referenceKeys must be unique")
            if not isinstance(entry.get("allowed"), bool):
                errors.append(f"reuse.{target_id}.allowed must be boolean")
            if entry.get("reviewStatus") not in {"single", "unreviewed", "reviewed"}:
                errors.append(f"reuse.{target_id}.reviewStatus is invalid")
            elif len(references) > 1 and entry["allowed"] and entry["reviewStatus"] != "reviewed":
                errors.append(f"reuse.{target_id} cannot allow unreviewed multi-test reuse")
            elif len(references) == 1 and entry["reviewStatus"] != "single":
                errors.append(f"reuse.{target_id} single-test reuse must have reviewStatus=single")
            if set(references) != set(expected_reuse.get(target_id, [])):
                errors.append(f"reuse.{target_id}.referenceKeys do not match rustEvidence")
        for target_id in expected_reuse:
            if target_id not in reuse:
                errors.append(f"reuse is missing {target_id}")

    return errors


def strict_errors(mappings: dict[str, dict[str, Any]], reuse: dict[str, Any]) -> list[str]:
    """Return errors that prevent a row from being treated as release evidence."""

    errors: list[str] = []
    for key, item in mappings.items():
        if item.get("evidence_type") != "function_exact":
            continue
        if not item.get("rustEvidence"):
            errors.append(f"{key}: function_exact has no Rust evidence")
        elif any(not target.get("anchor") for target in item["rustEvidence"]):
            errors.append(f"{key}: function_exact has Rust evidence without a parity anchor")
        for target in item.get("rustEvidence", []):
            reuse_key = f"{target['file']}::{target['test']}"
            relation = reuse.get(reuse_key, {})
            if relation.get("referenceCount", 0) > 1 and (
                not relation.get("allowed") or relation.get("reviewStatus") != "reviewed"
            ):
                errors.append(f"{key}: function_exact reuses {reuse_key} without a reviewed relation")
        coverage = item.get("assertionCoverage", {})
        if not item.get("coveredAssertions") or item.get("uncoveredAssertions") or coverage.get("source") != "reviewed":
            errors.append(f"{key}: function_exact assertion coverage is not reviewed")
        verification = item.get("verification", {})
        evidence_tests = {target["test"] for target in item.get("rustEvidence", [])}
        if not evidence_tests or not evidence_tests.issubset(set(verification.get("testFilter", []))):
            errors.append(f"{key}: function_exact has no executable test filter")
        verified_at = verification.get("verifiedAt")
        try:
            timestamp_valid = isinstance(verified_at, str) and bool(_datetime.datetime.fromisoformat(verified_at.replace("Z", "+00:00")).tzinfo)
        except ValueError:
            timestamp_valid = False
        digest = verification.get("receiptDigest")
        if (
            verification.get("status") != "passed"
            or verification.get("exitCode") != 0
            or not verification.get("argv")
            or not verification.get("runnerVersion")
            or not verification.get("toolchain")
            or not _SHA1.fullmatch(verification.get("verifiedCommit") or "")
            or not timestamp_valid
            or not (isinstance(digest, str) and digest.startswith("sha256:") and _SHA256.fullmatch(digest[7:]))
        ):
            errors.append(f"{key}: function_exact lacks a passed verification receipt")
    return errors
