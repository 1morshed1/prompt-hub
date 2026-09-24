# 07 — The Python SDK

The SDK is PromptHub's **adoption surface**. If wiring it into an app is harder
than a hardcoded string, nobody switches — so the whole design goal is: make the
common path a genuine one-liner, and add nothing to the user's dependency tree.
Code: `sdk/python/prompthub/`.

## The two calls that matter

Ninety percent of usage is these two:

```python
rendered = hub.get("order-assistant", tag="production").render(query="...", role="...")
# ... call your LLM with rendered.messages ...
hub.log_result(rendered.request_id, rating=1, latency_ms=1200)
```

Everything else (creating prompts, tagging, experiments) is management tooling
layered on top. Getting these two right is the job.

## Design decisions

### 1. Rendering happens on the server, not in the SDK

The template engine lives in the Go server. The SDK's `render()` **POSTs the
variables** and gets back finished `system`/`user` messages. Why not template
client-side?

- **One engine, one behavior.** If the SDK re-implemented templating, Python and
  Go could disagree on an edge case and you'd render different prompts in
  dev vs. prod. Server-side rendering means the platform's recorded version *is*
  what runs.
- **Analytics for free.** Because every render is a server call, the platform
  records which version served each request — the data the dashboard shows —
  without the SDK doing anything special.

### 2. Split `get()` from `render()`

`hub.get(name, tag=...)` returns a lightweight `Prompt` handle; `.render(**vars)`
does the network call. This mirrors the plan's ergonomics and lets you hold one
handle and render it many times (e.g. per request in a loop) without repeating the
selector.

### 3. `rendered.messages` returns chat-ready output

The `RenderedPrompt.messages` property returns `[{"role": "system", ...}, {"role":
"user", ...}]`, skipping an empty system prompt. That's the exact shape every chat
LLM SDK wants, so the handoff is `my_llm.chat(rendered.messages)` with no glue.

### 4. Zero runtime dependencies

The transport is stdlib `urllib`, not `requests`. A client library you drop into
someone's app should not drag in a dependency tree — version conflicts with the
host app are a real adoption blocker. The transport surface is tiny (one
`_request` method), so if you'd rather use `requests`, swapping it is trivial. The
trade-off (urllib's API is clunkier than `requests`) is entirely hidden inside
`_request`.

### 5. HTTP status codes → typed exceptions

`_request` maps 404 → `NotFoundError`, 422 → `ValidationError`, everything else →
`APIError` (which carries the status and body). Callers `except ValidationError`
to show a user their bad input, without inspecting status codes. Unreachable server
also becomes `APIError`, so network failure and API failure are one `except`.

### 6. `log_result` sends only what you give it

Metrics arrive at different times: latency is known immediately, the user's
thumbs-up maybe minutes later. `log_result` includes only the fields you pass, and
the server merges them into the request's row (`COALESCE`), so you can call it
twice — once with latency, once with the rating — against the same `request_id`.
Extra keyword args become `custom_metrics`, so app-specific scores need no SDK
change.

## Testing without a server

`tests/test_client.py` spins up a stdlib `http.server` that records requests and
returns canned responses. That exercises the *real* urllib transport, URL
building, project-param injection, error mapping, and the value objects — all
offline, in milliseconds. The tests assert, for example, that `log_result` omits
fields you didn't pass and that a 422 surfaces as `ValidationError`.

```bash
cd sdk/python && PYTHONPATH=. python -m pytest tests/ -q
```

## Try it against a real server

With the server running (see doc 01) and `scripts/seed.py` having created data:

```python
from prompthub import PromptHub
hub = PromptHub("http://localhost:8080")
r = hub.get("order-assistant", tag="production").render(
    role="support", topic="refunds", query="Where is my order?")
print(r.messages)
hub.log_result(r.request_id, rating=1, latency_ms=900)
```
