#!/usr/bin/env python3
"""End-to-end demo for secure-agent-platform (stdlib only).

Logs in as two demo tenants via OIDC (authorization-code + PKCE), runs an agent,
prints the streamed event trace and the audit log, and proves that one tenant
cannot see another's run.
"""

from __future__ import annotations

import base64
import hashlib
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

API = os.environ.get("SAP_API", "http://localhost:8080")
OIDC = os.environ.get("SAP_OIDC", "http://localhost:9000")
REDIRECT = "http://localhost:5173/callback"
CLIENT_ID = "sap-dashboard"


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *_args, **_kwargs):  # noqa: D401, ANN
        return None


def _b64url(raw: bytes) -> str:
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode()


def login(user: str) -> str:
    """Run the full auth-code + PKCE flow for a demo user; return the access token."""
    verifier = _b64url(os.urandom(32))
    challenge = _b64url(hashlib.sha256(verifier.encode()).digest())

    query = urllib.parse.urlencode(
        {
            "response_type": "code",
            "client_id": CLIENT_ID,
            "redirect_uri": REDIRECT,
            "scope": "openid profile email",
            "state": "demo",
            "code_challenge": challenge,
            "code_challenge_method": "S256",
            "login": user,  # scripted login shortcut supported by the mock IdP
        }
    )
    opener = urllib.request.build_opener(_NoRedirect)
    location = ""
    try:
        resp = opener.open(f"{OIDC}/authorize?{query}")
        location = resp.headers.get("Location", "")
    except urllib.error.HTTPError as exc:
        location = exc.headers.get("Location", "")
    if not location:
        raise SystemExit("no redirect Location returned from /authorize")
    code = urllib.parse.parse_qs(urllib.parse.urlparse(location).query).get("code", [""])[0]
    if not code:
        raise SystemExit(f"no authorization code in redirect: {location}")

    data = urllib.parse.urlencode(
        {
            "grant_type": "authorization_code",
            "code": code,
            "redirect_uri": REDIRECT,
            "client_id": CLIENT_ID,
            "code_verifier": verifier,
        }
    ).encode()
    with urllib.request.urlopen(urllib.request.Request(f"{OIDC}/token", data=data)) as resp:
        return json.load(resp)["access_token"]


def api(method: str, path: str, token: str, body: dict | None = None) -> tuple[int, object]:
    req = urllib.request.Request(f"{API}{path}", method=method)
    req.add_header("Authorization", f"Bearer {token}")
    if body is not None:
        req.data = json.dumps(body).encode()
        req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req) as resp:
            payload = resp.read()
            return resp.status, (json.loads(payload) if payload else None)
    except urllib.error.HTTPError as exc:
        return exc.code, exc.read().decode(errors="replace")


def sse(path: str, token: str) -> list[dict]:
    """Read a Server-Sent-Events stream until the 'done' event."""
    req = urllib.request.Request(f"{API}{path}")
    req.add_header("Authorization", f"Bearer {token}")
    events: list[dict] = []
    with urllib.request.urlopen(req) as resp:
        kind = ""
        for raw in resp:
            line = raw.decode().rstrip("\n")
            if line.startswith("event:"):
                kind = line.split(":", 1)[1].strip()
            elif line.startswith("data:"):
                data = line.split(":", 1)[1].strip()
                if kind == "done":
                    break
                if data:
                    events.append(json.loads(data))
    return events


def hr(title: str) -> None:
    print(f"\n\033[1;36m== {title} ==\033[0m")


def main() -> None:
    hr("Log in as Alice (Tenant A) and Bob (Tenant B) via OIDC + PKCE")
    alice = login("alice")
    bob = login("bob")
    _, alice_me = api("GET", "/api/v1/me", alice)
    _, bob_me = api("GET", "/api/v1/me", bob)
    print(f"  alice -> tenant {alice_me['tenantId']} roles={alice_me['roles']}")
    print(f"  bob   -> tenant {bob_me['tenantId']} roles={bob_me['roles']}")

    hr("Alice starts an agent run")
    status, run = api(
        "POST",
        "/api/v1/runs",
        alice,
        {"objective": "Summarize the platform's security model", "allowedTools": ["echo"]},
    )
    run_id = run["id"]
    print(f"  created run {run_id} (status={run['status']})")

    hr("Live event trace (streamed from the worker via the control plane)")
    # Give the async dispatch a moment, then read the (replayed/live) trace.
    time.sleep(1.5)
    for ev in sse(f"/api/v1/runs/{run_id}/events", alice):
        print(f"  [{ev['kind']:<14}] {json.dumps(ev['payload'])}")

    _, final = api("GET", f"/api/v1/runs/{run_id}", alice)
    print(f"\n  final status: {final['status']}")
    print(f"  final answer: {final['finalAnswer']}")

    hr("Audit log (Alice's tenant) — least-privilege tool proxy in action")
    _, audit = api("GET", "/api/v1/audit", alice)
    for a in audit[:12]:
        tool = f" tool={a['toolName']}" if a["toolName"] else ""
        print(f"  {a['at']}  {a['actor']:<40} {a['action']}{tool}")

    hr("Tenant isolation: Bob tries to read Alice's run")
    code, _ = api("GET", f"/api/v1/runs/{run_id}", bob)
    if code == 404:
        print(f"  Bob GET /runs/{run_id} -> 404 Not Found  ✅ isolation holds")
    else:
        print(f"  Bob GET /runs/{run_id} -> {code}  ❌ EXPECTED 404")
        sys.exit(1)

    print("\n\033[1;32mDemo complete.\033[0m Open the dashboard at http://localhost:5173\n")


if __name__ == "__main__":
    main()
