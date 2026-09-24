"""Unit tests for the SDK against a stub HTTP server.

We spin up a tiny stdlib ``http.server`` that records requests and returns canned
responses, so these tests exercise the real urllib transport, URL building, error
mapping, and value objects — without needing a running PromptHub server.
"""

import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import pytest

from prompthub import PromptHub, NotFoundError, ValidationError, APIError


class _Handler(BaseHTTPRequestHandler):
    # Populated per-test via the server instance.
    def log_message(self, *args):  # silence test noise
        pass

    def _send(self, status, obj):
        body = json.dumps(obj).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(body)

    def _read_body(self):
        length = int(self.headers.get("Content-Length", 0))
        raw = self.rfile.read(length) if length else b""
        return json.loads(raw) if raw else {}

    def do_GET(self):
        self.server.requests.append(("GET", self.path, {}))
        if self.path.startswith("/api/v1/prompts") and "/diff" in self.path:
            self._send(200, {"from": 1, "to": 2, "mode": "line"})
        elif self.path == "/missing":
            self._send(404, {"error": "not found"})
        else:
            self._send(200, {"ok": True})

    def do_PUT(self):
        body = self._read_body()
        self.server.requests.append(("PUT", self.path, body))
        self._send(200, {"tag": "production", "version_id": "v-id"})

    def do_POST(self):
        body = self._read_body()
        self.server.requests.append(("POST", self.path, body))
        if self.path.endswith("/render"):
            # Echo back a rendered payload; include experiment fields.
            self._send(200, {
                "prompt_name": "order-assistant",
                "version_number": 2,
                "system": "You are a support assistant.",
                "user": f"About {body['variables'].get('topic')}: {body['variables'].get('query')}",
                "model_config": {"model": "gpt-4o", "temperature": 0.3},
                "request_id": "req-123",
                "experiment_id": "exp-1",
                "arm": "variant",
            })
        elif self.path == "/api/v1/results":
            self._send(200, {"status": "logged"})
        elif "/render-invalid" in self.path:
            self._send(422, {"error": "variable validation failed: \"query\" is required"})
        else:
            self._send(201, {"created": True, "echo": body})


@pytest.fixture
def server():
    srv = ThreadingHTTPServer(("127.0.0.1", 0), _Handler)
    srv.requests = []
    t = threading.Thread(target=srv.serve_forever, daemon=True)
    t.start()
    host, port = srv.server_address
    yield srv, f"http://{host}:{port}"
    srv.shutdown()


def test_render_returns_messages(server):
    srv, base = server
    hub = PromptHub(base_url=base)
    r = hub.get("order-assistant", tag="production").render(
        role="support", topic="refunds", query="Where is my order?"
    )
    assert r.version_number == 2
    assert r.request_id == "req-123"
    assert r.experiment_id == "exp-1"
    assert r.arm == "variant"
    assert r.user == "About refunds: Where is my order?"
    # messages() builds a proper chat list.
    msgs = r.messages
    assert msgs[0]["role"] == "system"
    assert msgs[1] == {"role": "user", "content": "About refunds: Where is my order?"}


def test_render_sends_selectors(server):
    srv, base = server
    hub = PromptHub(base_url=base)
    hub.get("p", version=3).render(unit_id="user-1", x="y")
    method, path, body = srv.requests[-1]
    assert method == "POST" and path.endswith("/p/render")
    assert body["version"] == 3
    assert body["unit_id"] == "user-1"
    assert body["variables"] == {"x": "y"}


def test_log_result_only_sends_provided_fields(server):
    srv, base = server
    hub = PromptHub(base_url=base)
    hub.log_result("req-123", rating=1, latency_ms=900, quality=0.8)
    _, path, body = srv.requests[-1]
    assert path == "/api/v1/results"
    assert body["request_id"] == "req-123"
    assert body["rating"] == 1
    assert body["latency_ms"] == 900
    assert body["custom_metrics"] == {"quality": 0.8}
    assert "cost_usd" not in body  # not provided -> not sent


def test_project_query_param(server):
    srv, base = server
    hub = PromptHub(base_url=base, project="customer-support")
    hub.list_prompts()
    _, path, _ = srv.requests[-1]
    assert "project=customer-support" in path


def test_not_found_maps_to_exception(server):
    srv, base = server
    hub = PromptHub(base_url=base)
    with pytest.raises(NotFoundError):
        hub._request("GET", "/missing")


def test_validation_error_maps(server):
    srv, base = server
    hub = PromptHub(base_url=base)
    with pytest.raises(ValidationError):
        hub._request("POST", "/render-invalid", {"variables": {}})


def test_unreachable_server_raises_apierror():
    hub = PromptHub(base_url="http://127.0.0.1:1", timeout=0.5)  # nothing listening
    with pytest.raises(APIError):
        hub.list_prompts()
