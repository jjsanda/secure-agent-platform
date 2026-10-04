"""SSRF / private-egress guard — worker-local defense-in-depth.

Inspects URL-shaped tool arguments and rejects targets that resolve to
loopback, private, link-local, unspecified, multicast, or cloud-metadata
ranges (169.254.169.254 and the IPv6 ``fd00:ec2::254`` variant) before the call
ever leaves the worker. Only ``http`` / ``https`` schemes are allowed.

This mirrors the authoritative Go check in ``control-plane/internal/guard`` — the
proxy remains the boundary that counts, but a defense-in-depth worker check
means the worker never even attempts an obviously-unsafe fetch. :func:`resolve`
returns the resolved IPs so a caller could pin the connection to them and defeat
DNS rebinding, exactly as the Go ``http_get`` tool does.
"""

from __future__ import annotations

import ipaddress
import socket
from collections.abc import Mapping
from typing import Any
from urllib.parse import urlsplit

from sap_worker.exceptions import GuardBlocked

__all__ = [
    "URL_ARGUMENT_KEYS",
    "IPAddress",
    "SsrfBlocked",
    "is_blocked_ip",
    "resolve",
    "check",
]

IPAddress = ipaddress.IPv4Address | ipaddress.IPv6Address

# Argument keys whose string value is treated as a URL and validated. Kept in
# sync with the guarded ``http_get`` tool (``{"url": ...}``) plus common aliases.
URL_ARGUMENT_KEYS: frozenset[str] = frozenset(
    {"url", "uri", "endpoint", "callback", "callback_url", "webhook", "href", "link"}
)

_ALLOWED_SCHEMES: frozenset[str] = frozenset({"http", "https"})

# Cloud instance-metadata endpoints. 169.254.169.254 is already link-local and
# fd00:ec2::254 is already private, but block them explicitly to state intent.
_METADATA_IPS: frozenset[IPAddress] = frozenset(
    {
        ipaddress.ip_address("169.254.169.254"),
        ipaddress.ip_address("fd00:ec2::254"),
    }
)


class SsrfBlocked(GuardBlocked):
    """A URL argument targeted a non-public / non-http(s) destination."""

    def __init__(self, reason: str, *, tool_name: str | None = None) -> None:
        super().__init__("ssrf", reason, tool_name=tool_name)


def is_blocked_ip(ip: IPAddress) -> bool:
    """Report whether ``ip`` is one an agent must never be able to reach."""
    return bool(
        ip.is_loopback
        or ip.is_private
        or ip.is_unspecified
        or ip.is_link_local
        or ip.is_multicast
        or ip in _METADATA_IPS
    )


def resolve(raw_url: str, *, tool_name: str | None = None) -> list[IPAddress]:
    """Validate ``raw_url`` and return the public IPs its host resolves to.

    Raises :class:`SsrfBlocked` if the scheme is not http(s), the host is
    missing or unresolvable, or *any* resolved address is in a blocked range.
    Returning the full set (not just the first) lets a caller pin every
    candidate address.
    """
    parts = urlsplit(raw_url)
    if parts.scheme.lower() not in _ALLOWED_SCHEMES:
        raise SsrfBlocked(f"scheme {parts.scheme!r} is not allowed", tool_name=tool_name)
    host = parts.hostname
    if not host:
        raise SsrfBlocked("url has no host", tool_name=tool_name)

    try:
        infos = socket.getaddrinfo(host, parts.port, proto=socket.IPPROTO_TCP)
    except socket.gaierror as exc:
        raise SsrfBlocked(f"cannot resolve host {host!r}", tool_name=tool_name) from exc

    resolved: list[IPAddress] = []
    seen: set[IPAddress] = set()
    for info in infos:
        sockaddr = info[4]
        ip = ipaddress.ip_address(sockaddr[0])
        if ip in seen:
            continue
        seen.add(ip)
        resolved.append(ip)

    if not resolved:
        raise SsrfBlocked(f"cannot resolve host {host!r}", tool_name=tool_name)
    for ip in resolved:
        if is_blocked_ip(ip):
            raise SsrfBlocked(
                f"host {host!r} resolves to blocked address {ip}", tool_name=tool_name
            )
    return resolved


def check(tool_name: str, arguments: Mapping[str, Any]) -> None:
    """Validate every URL-shaped argument of ``tool_name``; raise on the first bad one."""
    for key, value in arguments.items():
        if key in URL_ARGUMENT_KEYS and isinstance(value, str):
            resolve(value, tool_name=tool_name)
