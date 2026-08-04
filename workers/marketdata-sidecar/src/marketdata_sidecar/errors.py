"""Stable sidecar error types shared by all routes."""

from __future__ import annotations


class SidecarError(Exception):
    """An expected error that is safe to expose over the local HTTP contract."""

    def __init__(self, status_code: int, code: str, message: str) -> None:
        super().__init__(message)
        self.status_code = status_code
        self.code = code
        self.message = message


def invalid_request(code: str, message: str) -> SidecarError:
    return SidecarError(400, code, message)


def not_found(code: str, message: str) -> SidecarError:
    return SidecarError(404, code, message)


def upstream_error(message: str) -> SidecarError:
    return SidecarError(502, "upstream_error", message)


def service_unavailable(code: str, message: str) -> SidecarError:
    return SidecarError(503, code, message)
