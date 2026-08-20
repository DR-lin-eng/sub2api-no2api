#!/usr/bin/env python3

import json
import os
import threading
import unittest
from argparse import Namespace
from contextlib import redirect_stdout
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from io import StringIO
from unittest.mock import patch
from urllib.parse import parse_qs, urlparse

from tools.disable_oauth_accounts import (
    AdminAPI,
    ScriptError,
    account_quota_exhaustion_reason,
    apply_candidates,
    candidate_from_item,
    list_accounts,
    run,
)


class _MockHandler(BaseHTTPRequestHandler):
    requests = []

    def log_message(self, _format, *_args):
        return

    def _write(self, payload, status=200):
        raw = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def do_GET(self):
        parsed = urlparse(self.path)
        query = parse_qs(parsed.query)
        self.__class__.requests.append(("GET", parsed.path, query, self.headers.get("x-api-key")))
        if query.get("oauth_quota") == ["exhausted"]:
            items = [{"id": 2, "name": "quota", "platform": "openai", "type": "oauth", "schedulable": True}]
        else:
            items = [
                {
                    "id": 1,
                    "name": "slow",
                    "platform": "openai",
                    "type": "oauth",
                    "schedulable": True,
                    "hourly_usage": {
                        "total_requests": 2,
                        "avg_first_token_ms": 30_001,
                        "success_rate": 1,
                    },
                },
                {
                    "id": 2,
                    "name": "quota",
                    "platform": "openai",
                    "type": "oauth",
                    "schedulable": True,
                    "hourly_usage": {
                        "total_requests": 4,
                        "avg_first_token_ms": 100,
                        "success_rate": 1,
                    },
                },
            ]
        self._write(
            {
                "code": 0,
                "message": "success",
                "data": {"items": items, "total": len(items), "pages": 1},
            }
        )

    def do_POST(self):
        parsed = urlparse(self.path)
        self.__class__.requests.append(("POST", parsed.path, {}, self.headers.get("x-api-key")))
        self._write({"code": 0, "message": "success", "data": {"id": 1, "schedulable": False}})


class DisableOAuthAccountsTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        _MockHandler.requests = []
        cls.server = ThreadingHTTPServer(("127.0.0.1", 0), _MockHandler)
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()
        cls.base_url = f"http://127.0.0.1:{cls.server.server_port}"

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.thread.join(timeout=2)
        cls.server.server_close()

    def setUp(self):
        _MockHandler.requests = []
        self.api = AdminAPI(self.base_url, "test-key", timeout=2, retries=0)

    def test_candidate_thresholds_and_quota_are_or_conditions(self):
        slow = candidate_from_item(
            {
                "id": 1,
                "name": "slow",
                "platform": "openai",
                "schedulable": True,
                "hourly_usage": {"total_requests": 1, "avg_first_token_ms": 30_001, "success_rate": 1},
            },
            set(),
            ttft_limit_ms=30_000,
            success_rate_limit=0.6,
            min_requests=1,
        )
        self.assertIsNotNone(slow)
        self.assertIn("avg_first_token_ms>30000", slow.reasons)

        unreliable = candidate_from_item(
            {
                "id": 5,
                "name": "unreliable",
                "platform": "openai",
                "schedulable": True,
                "hourly_usage": {"total_requests": 10, "avg_first_token_ms": 100, "success_rate": 0.59},
            },
            set(),
            ttft_limit_ms=30_000,
            success_rate_limit=0.6,
            min_requests=1,
        )
        self.assertIsNotNone(unreliable)
        self.assertIn("success_rate<60%", unreliable.reasons)

        quota = candidate_from_item(
            {
                "id": 2,
                "name": "quota",
                "platform": "openai",
                "schedulable": True,
                "hourly_usage": {"total_requests": 0, "avg_first_token_ms": None, "success_rate": 0},
            },
            {2},
            ttft_limit_ms=30_000,
            success_rate_limit=0.6,
            min_requests=1,
        )
        self.assertEqual(quota.reasons, ("oauth_quota_exhausted",))

    def test_empty_window_does_not_trigger_low_success_rate(self):
        candidate = candidate_from_item(
            {
                "id": 3,
                "name": "idle",
                "platform": "openai",
                "schedulable": True,
                "hourly_usage": {"total_requests": 0, "avg_first_token_ms": None, "success_rate": 0},
            },
            set(),
            ttft_limit_ms=30_000,
            success_rate_limit=0.6,
            min_requests=1,
        )
        self.assertIsNone(candidate)

    def test_quota_snapshot_requires_threshold_and_active_reset(self):
        self.assertEqual(
            account_quota_exhaustion_reason(
                {
                    "extra": {
                        "codex_7d_used_percent": 100,
                        "codex_7d_reset_at": "2099-01-01T00:00:00Z",
                    }
                }
            ),
            "codex_7d_used_percent",
        )
        self.assertIsNone(
            account_quota_exhaustion_reason(
                {
                    "extra": {
                        "codex_7d_used_percent": 100,
                        "codex_7d_reset_at": "2000-01-01T00:00:00Z",
                    }
                }
            )
        )

    def test_missing_hourly_usage_fails_closed(self):
        with self.assertRaises(ScriptError):
            candidate_from_item(
                {"id": 4, "name": "missing", "schedulable": True},
                set(),
                ttft_limit_ms=30_000,
                success_rate_limit=0.6,
                min_requests=1,
            )

    def test_list_and_apply_use_admin_key_and_schedulable_endpoint(self):
        accounts = list_accounts(
            self.api,
            platform="openai",
            page_size=100,
            include_hourly_usage=True,
        )
        exhausted = list_accounts(
            self.api,
            platform="openai",
            page_size=100,
            include_hourly_usage=False,
            exhausted_only=True,
        )
        self.assertEqual([item["id"] for item in accounts], [1, 2])
        self.assertEqual([item["id"] for item in exhausted], [2])
        candidate = candidate_from_item(
            accounts[0],
            {2},
            ttft_limit_ms=30_000,
            success_rate_limit=0.6,
            min_requests=1,
        )
        results = apply_candidates(self.api, [candidate])
        self.assertEqual(results, [{"account_id": 1, "status": "disabled"}])
        self.assertEqual(_MockHandler.requests[-1][0:2], ("POST", "/api/v1/admin/accounts/1/schedulable"))
        get_requests = [entry for entry in _MockHandler.requests if entry[0] == "GET"]
        self.assertTrue(all(entry[2].get("sort_by") == ["id"] for entry in get_requests))
        self.assertTrue(all(entry[3] == "test-key" for entry in _MockHandler.requests))

    def test_run_is_dry_run_without_apply(self):
        args = Namespace(
            base_url=self.base_url,
            platform="openai",
            page_size=100,
            ttft_ms=30_000,
            success_rate=0.6,
            min_requests=1,
            timeout=2,
            apply=False,
        )
        output = StringIO()
        with patch.dict(os.environ, {"SUB2API_ADMIN_API_KEY": "test-key"}), redirect_stdout(output):
            self.assertEqual(run(args), 0)
        self.assertIn("Dry-run only", output.getvalue())
        self.assertFalse(any(entry[0] == "POST" for entry in _MockHandler.requests))


if __name__ == "__main__":
    unittest.main()
