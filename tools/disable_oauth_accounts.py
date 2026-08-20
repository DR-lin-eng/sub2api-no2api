#!/usr/bin/env python3
"""Disable unhealthy OAuth accounts from Sub2API scheduling.

The script is dry-run by default. It uses the Admin API's rolling-hour account
metrics and locally verifies persisted OAuth quota snapshots and reset times.
It only disables scheduling; it never re-enables an account.
"""

from __future__ import annotations

import argparse
import json
import math
import os
import ssl
import sys
import time
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Any
from urllib.error import HTTPError, URLError
from urllib.parse import urlencode, urlparse
from urllib.request import Request, urlopen


DEFAULT_PAGE_SIZE = 100
DEFAULT_TTFT_LIMIT_MS = 30_000.0
DEFAULT_SUCCESS_RATE_LIMIT = 0.60
DEFAULT_TIMEOUT_SECONDS = 20.0
API_PATH = "/api/v1/admin/accounts"
API_KEY_ENV = "SUB2API_ADMIN_API_KEY"


def verified_ssl_context() -> ssl.SSLContext:
    """Use certifi or a system bundle while keeping TLS verification enabled."""

    candidates: list[str] = []
    try:
        import certifi

        candidates.append(certifi.where())
    except (ImportError, AttributeError):
        pass
    default_cafile = ssl.get_default_verify_paths().cafile
    if default_cafile:
        candidates.append(default_cafile)
    candidates.append("/etc/ssl/cert.pem")
    for cafile in candidates:
        if os.path.isfile(cafile):
            try:
                return ssl.create_default_context(cafile=cafile)
            except ssl.SSLError:
                continue
    return ssl.create_default_context()


class ScriptError(RuntimeError):
    """Expected operational or validation failure."""


class APIError(ScriptError):
    def __init__(self, message: str, status: int | None = None) -> None:
        super().__init__(message)
        self.status = status


@dataclass(frozen=True)
class Candidate:
    account_id: int
    name: str
    platform: str
    schedulable: bool
    total_requests: int
    avg_first_token_ms: float | None
    success_rate: float | None
    reasons: tuple[str, ...]


class AdminAPI:
    """Dependency-free client for the Sub2API Admin API."""

    def __init__(
        self,
        base_url: str,
        api_key: str,
        timeout: float,
        retries: int = 2,
        ssl_context: ssl.SSLContext | None = None,
    ) -> None:
        parsed = urlparse(base_url)
        if parsed.scheme not in {"http", "https"} or not parsed.netloc:
            raise ScriptError(f"invalid base URL: {base_url!r}")
        if parsed.username or parsed.password or parsed.query or parsed.fragment:
            raise ScriptError("base URL must not contain credentials, a query, or a fragment")
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key
        self.timeout = timeout
        self.retries = retries
        self.ssl_context = ssl_context or verified_ssl_context()

    def get(self, path: str, params: dict[str, Any]) -> dict[str, Any]:
        query = urlencode([(key, str(value)) for key, value in params.items()])
        return self._request("GET", f"{path}?{query}")

    def post(self, path: str, payload: dict[str, Any]) -> dict[str, Any]:
        return self._request("POST", path, payload)

    def _request(
        self,
        method: str,
        path: str,
        payload: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        body = None
        if payload is not None:
            body = json.dumps(payload, separators=(",", ":")).encode("utf-8")
        headers = {
            "Accept": "application/json",
            "User-Agent": "sub2api-oauth-scheduler/1.0",
            "x-api-key": self.api_key,
        }
        if body is not None:
            headers["Content-Type"] = "application/json"

        attempts = self.retries + 1 if method == "GET" else 1
        for attempt in range(attempts):
            request = Request(
                f"{self.base_url}{path}",
                data=body,
                headers=headers,
                method=method,
            )
            try:
                with urlopen(request, timeout=self.timeout, context=self.ssl_context) as response:
                    return self._decode_response(response.status, response.read(), method, path)
            except HTTPError as error:
                raw = error.read()
                if method == "GET" and error.code in {429, 500, 502, 503, 504} and attempt + 1 < attempts:
                    time.sleep(min(0.5 * (2**attempt), 2.0))
                    continue
                raise APIError(self._error_message(error.code, raw), error.code) from error
            except (URLError, TimeoutError, OSError) as error:
                if method == "GET" and attempt + 1 < attempts:
                    time.sleep(min(0.5 * (2**attempt), 2.0))
                    continue
                reason = getattr(error, "reason", error)
                raise APIError(f"request failed for {method} {path.split('?', 1)[0]}: {reason}") from error

        raise APIError(f"request failed for {method} {path.split('?', 1)[0]}")

    @staticmethod
    def _decode_response(status: int, raw: bytes, method: str, path: str) -> dict[str, Any]:
        if status < 200 or status >= 300:
            raise APIError(AdminAPI._error_message(status, raw), status)
        try:
            decoded = json.loads(raw.decode("utf-8"))
        except (UnicodeDecodeError, json.JSONDecodeError) as error:
            raise APIError(f"Admin API returned invalid JSON for {method} {path.split('?', 1)[0]}") from error
        if not isinstance(decoded, dict):
            raise APIError(f"Admin API returned a non-object response for {method} {path.split('?', 1)[0]}")
        return decoded

    @staticmethod
    def _error_message(status: int, raw: bytes) -> str:
        message = ""
        try:
            decoded = json.loads(raw.decode("utf-8"))
            if isinstance(decoded, dict):
                message = str(decoded.get("message") or decoded.get("error") or "")
        except (UnicodeDecodeError, json.JSONDecodeError):
            pass
        message = " ".join(message.split())[:300]
        return f"Admin API returned HTTP {status}" + (f": {message}" if message else "")


def unwrap_data(response: dict[str, Any]) -> Any:
    return response.get("data", response)


def list_accounts(
    api: AdminAPI,
    *,
    platform: str,
    page_size: int,
    include_hourly_usage: bool,
    exhausted_only: bool = False,
) -> list[dict[str, Any]]:
    """Read all matching accounts using stable ID-ordered pagination."""

    items: list[dict[str, Any]] = []
    page = 1
    expected_total: int | None = None
    while True:
        params: dict[str, Any] = {
            "page": page,
            "page_size": page_size,
            "type": "oauth",
            "include_hourly_usage": 1 if include_hourly_usage else 0,
            "sort_by": "id",
            "sort_order": "asc",
        }
        if platform:
            params["platform"] = platform
        if exhausted_only:
            params["oauth_quota"] = "exhausted"

        data = unwrap_data(api.get(API_PATH, params))
        if not isinstance(data, dict) or not isinstance(data.get("items"), list):
            raise ScriptError("account list response is missing its pagination data")
        page_items = data["items"]
        if not all(isinstance(item, dict) for item in page_items):
            raise ScriptError("account list contains a non-object item")
        items.extend(page_items)

        try:
            expected_total = int(data.get("total", len(items)))
            pages = int(data.get("pages", 0))
        except (TypeError, ValueError) as error:
            raise ScriptError("account list has invalid pagination metadata") from error
        done = page >= pages if pages > 0 else len(page_items) < page_size
        if done:
            break
        page += 1
        if page > 10_000:
            raise ScriptError("account pagination exceeded the safety limit")

    if expected_total is not None and len(items) != expected_total:
        raise ScriptError(f"account pagination was inconsistent: received {len(items)} of {expected_total}")
    return items


def account_id(item: dict[str, Any]) -> int:
    value = item.get("id")
    if isinstance(value, bool) or not isinstance(value, int) or value <= 0:
        raise ScriptError("account list contains an invalid account id")
    return value


def finite_float(value: Any) -> float | None:
    if value is None or isinstance(value, bool):
        return None
    try:
        parsed = float(value)
    except (TypeError, ValueError):
        return None
    return parsed if math.isfinite(parsed) else None


def quota_reset_active(value: Any, *, unix_seconds: bool = False) -> bool:
    """Match the backend's fail-closed reset handling for quota snapshots."""

    if value is None or value == "":
        return True
    if unix_seconds:
        reset_at = finite_float(value)
        return reset_at is None or reset_at > time.time()
    if not isinstance(value, str) or not value.strip():
        return True
    try:
        timestamp = value.strip().replace("Z", "+00:00")
        parsed = datetime.fromisoformat(timestamp)
        if parsed.tzinfo is None:
            parsed = parsed.replace(tzinfo=timezone.utc)
        return parsed.timestamp() > time.time()
    except (ValueError, OverflowError, OSError):
        return True


def account_quota_exhaustion_reason(item: dict[str, Any]) -> str | None:
    """Return the first locally verifiable exhausted OAuth quota window."""

    extra = item.get("extra") if isinstance(item.get("extra"), dict) else {}
    percent_windows = (
        ("codex_5h_used_percent", "codex_5h_reset_at"),
        ("codex_7d_used_percent", "codex_7d_reset_at"),
        ("codex_primary_used_percent", "codex_primary_reset_at"),
        ("codex_secondary_used_percent", "codex_secondary_reset_at"),
    )
    for usage_key, reset_key in percent_windows:
        usage = finite_float(extra.get(usage_key))
        if usage is not None and usage >= 100 and quota_reset_active(extra.get(reset_key)):
            return usage_key

    ratio_windows = (
        ("session_window_utilization", "session_window_end", False),
        ("passive_usage_7d_utilization", "passive_usage_7d_reset", True),
        ("passive_usage_7d_oi_utilization", "passive_usage_7d_oi_reset", True),
    )
    for usage_key, reset_key, unix_seconds in ratio_windows:
        usage = finite_float(extra.get(usage_key))
        reset_value = item.get(reset_key) if reset_key == "session_window_end" else extra.get(reset_key)
        if usage is not None and usage >= 1 and quota_reset_active(reset_value, unix_seconds=unix_seconds):
            return usage_key

    billing = extra.get("grok_billing_snapshot")
    if isinstance(billing, dict):
        active = quota_reset_active(billing.get("period_end")) and quota_reset_active(
            billing.get("billing_period_end")
        )
        if active:
            for usage_key in ("usage_percent", "used_percent"):
                usage = finite_float(billing.get(usage_key))
                if usage is not None and usage >= 100:
                    return f"grok_billing_snapshot.{usage_key}"
    return None


def candidate_from_item(
    item: dict[str, Any],
    exhausted_ids: set[int],
    *,
    ttft_limit_ms: float,
    success_rate_limit: float,
    min_requests: int,
) -> Candidate | None:
    """Evaluate one account; missing or invalid metrics fail closed."""

    item_id = account_id(item)
    hourly = item.get("hourly_usage")
    if not isinstance(hourly, dict):
        raise ScriptError(
            f"account {item_id} has no hourly_usage; refusing to write from missing data"
        )
    total_raw = hourly.get("total_requests")
    if isinstance(total_raw, bool) or not isinstance(total_raw, int) or total_raw < 0:
        raise ScriptError(f"account {item_id} has invalid hourly total_requests")
    total_requests = total_raw

    ttft = finite_float(hourly.get("avg_first_token_ms"))
    success_rate = finite_float(hourly.get("success_rate"))
    if ttft is not None and ttft < 0:
        raise ScriptError(f"account {item_id} has a negative hourly avg_first_token_ms")
    if success_rate is not None and not 0 <= success_rate <= 1:
        raise ScriptError(f"account {item_id} has an out-of-range hourly success_rate")

    reasons: list[str] = []
    if total_requests >= min_requests:
        if ttft is not None and ttft > ttft_limit_ms:
            reasons.append(f"avg_first_token_ms>{ttft_limit_ms:g}")
        if success_rate is not None and success_rate < success_rate_limit:
            reasons.append(f"success_rate<{success_rate_limit:.0%}")
    if item_id in exhausted_ids:
        reasons.append("oauth_quota_exhausted")
    if not reasons:
        return None

    raw_name = str(item.get("name") or f"account-{item_id}")
    return Candidate(
        account_id=item_id,
        name=" ".join(raw_name.split())[:160],
        platform=str(item.get("platform") or ""),
        schedulable=item.get("schedulable") is True,
        total_requests=total_requests,
        avg_first_token_ms=ttft,
        success_rate=success_rate,
        reasons=tuple(reasons),
    )


def apply_candidates(
    api: AdminAPI,
    candidates: list[Candidate],
) -> list[dict[str, Any]]:
    """Disable each selected account and verify the returned state."""

    results: list[dict[str, Any]] = []
    for candidate in candidates:
        if not candidate.schedulable:
            results.append({"account_id": candidate.account_id, "status": "skipped_already_disabled"})
            continue
        try:
            data = unwrap_data(
                api.post(
                    f"{API_PATH}/{candidate.account_id}/schedulable",
                    {"schedulable": False},
                )
            )
            if not isinstance(data, dict) or data.get("schedulable") is not False:
                raise ScriptError("endpoint response did not confirm schedulable=false")
            results.append({"account_id": candidate.account_id, "status": "disabled"})
        except APIError as error:
            if error.status in {401, 403}:
                raise ScriptError(
                    f"cannot write account {candidate.account_id}: {error}; "
                    "the key needs admin.accounts.write"
                ) from error
            results.append({"account_id": candidate.account_id, "status": "failed", "error": str(error)})
        except ScriptError as error:
            results.append({"account_id": candidate.account_id, "status": "failed", "error": str(error)})
    return results


def render_candidate(candidate: Candidate) -> str:
    ttft = "unknown" if candidate.avg_first_token_ms is None else f"{candidate.avg_first_token_ms:.1f}ms"
    rate = "unknown" if candidate.success_rate is None else f"{candidate.success_rate:.1%}"
    state = "enabled" if candidate.schedulable else "already-disabled"
    return (
        f"- {candidate.account_id} {candidate.name!r} [{state}] "
        f"requests={candidate.total_requests} ttft={ttft} success_rate={rate} "
        f"reasons={', '.join(candidate.reasons)}"
    )


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description=(
            "Dry-run by default: disable OAuth accounts with rolling-hour TTFT over 30s, "
            "success rate below 60%, or exhausted OAuth quota."
        )
    )
    parser.add_argument(
        "--base-url",
        default=os.getenv("SUB2API_BASE_URL", ""),
        help="Sub2API origin, or set SUB2API_BASE_URL",
    )
    parser.add_argument("--platform", default="", help="optional platform filter, for example openai")
    parser.add_argument("--page-size", type=int, default=DEFAULT_PAGE_SIZE)
    parser.add_argument("--ttft-ms", type=float, default=DEFAULT_TTFT_LIMIT_MS)
    parser.add_argument("--success-rate", type=float, default=DEFAULT_SUCCESS_RATE_LIMIT)
    parser.add_argument("--min-requests", type=int, default=1)
    parser.add_argument("--timeout", type=float, default=DEFAULT_TIMEOUT_SECONDS)
    parser.add_argument("--apply", action="store_true", help="perform writes; otherwise only preview")
    return parser


def validate_args(args: argparse.Namespace) -> None:
    if not 1 <= args.page_size <= 1000:
        raise ScriptError("--page-size must be between 1 and 1000")
    if not math.isfinite(args.ttft_ms) or args.ttft_ms < 0:
        raise ScriptError("--ttft-ms must be a finite non-negative number")
    if not math.isfinite(args.success_rate) or not 0 <= args.success_rate <= 1:
        raise ScriptError("--success-rate must be between 0 and 1")
    if args.min_requests < 1:
        raise ScriptError("--min-requests must be at least 1")
    if not math.isfinite(args.timeout) or args.timeout <= 0:
        raise ScriptError("--timeout must be positive")


def run(args: argparse.Namespace) -> int:
    validate_args(args)
    api_key = os.getenv(API_KEY_ENV, "").strip()
    if not api_key:
        raise ScriptError(
            f"missing Admin API key; set {API_KEY_ENV} "
            "(requires admin.accounts.read and admin.accounts.write for --apply)"
        )

    platform = args.platform.strip()
    api = AdminAPI(args.base_url, api_key, args.timeout)
    accounts = list_accounts(
        api,
        platform=platform,
        page_size=args.page_size,
        include_hourly_usage=True,
    )
    exhausted_ids = {
        account_id(item)
        for item in accounts
        if account_quota_exhaustion_reason(item) is not None
    }
    candidates: list[Candidate] = []
    for item in accounts:
        candidate = candidate_from_item(
            item,
            exhausted_ids,
            ttft_limit_ms=args.ttft_ms,
            success_rate_limit=args.success_rate,
            min_requests=args.min_requests,
        )
        if candidate is not None:
            candidates.append(candidate)

    scope = platform or "all platforms"
    print(f"Scanned {len(accounts)} OAuth account(s) ({scope}); quota-exhausted: {len(exhausted_ids)}.")
    print(f"Candidates: {len(candidates)}")
    for candidate in candidates:
        print(render_candidate(candidate))
    if not args.apply:
        print("Dry-run only. Re-run with --apply to disable scheduling.")
        return 0

    results = apply_candidates(api, candidates)
    disabled = sum(item["status"] == "disabled" for item in results)
    skipped = sum(item["status"] == "skipped_already_disabled" for item in results)
    failed = [item for item in results if item["status"] == "failed"]
    print(f"Applied: disabled={disabled} already_disabled={skipped} failed={len(failed)}")
    for item in failed:
        print(f"! {item['account_id']}: {item.get('error', 'unknown error')}", file=sys.stderr)
    return 1 if failed else 0


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        return run(args)
    except ScriptError as error:
        print(f"error: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
