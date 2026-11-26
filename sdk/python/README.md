# PromptHub Python SDK

A tiny, dependency-free client for [PromptHub](../../README.md) — use
version-controlled, A/B-tested prompts from your LLM application in one line.

## Install

```bash
pip install -e sdk/python        # from this repo
# or, once published: pip install prompthub-sdk
```

The SDK uses only the Python standard library — installing it pulls in nothing.

## Use

```python
from prompthub import PromptHub

hub = PromptHub(base_url="http://localhost:8080", project="customer-support")

# Fetch + render the production version. Templating happens server-side, so you
# always get the exact text the platform recorded.
rendered = hub.get("order-assistant", tag="production").render(
    role="support agent",
    topic="refunds",
    query="Where is my order?",
    language="en",
)

# rendered.messages is ready for any chat LLM:
#   [{"role": "system", ...}, {"role": "user", ...}]
response = my_llm.chat(rendered.messages, **rendered.model_config)

# Report the outcome so PromptHub can track performance and run experiments.
hub.log_result(rendered.request_id, rating=1, latency_ms=1200, cost_usd=0.0012)
```

If a running experiment covers this prompt, `hub.get(...).render(...)` is
automatically assigned to an arm; `rendered.arm` and `rendered.experiment_id` tell
you which, and your `log_result` call feeds the experiment.

## Management helpers

The same client can script the platform (used by `scripts/seed.py`):

```python
hub.create_prompt("order-assistant", description="helps with orders")
hub.create_version(
    "order-assistant",
    system_template="You are a {{role}} assistant.",
    user_template="About {{topic}}: {{query}}",
    variables=[{"name": "role", "type": "string", "required": True}, ...],
)
hub.set_tag("order-assistant", "production", 1)  # deploy
hub.rollback("order-assistant", "production", 1)  # roll back (same op)
exp = hub.create_experiment(
    "order-assistant",
    control_version=1,
    variant_version=2,
    auto_promote=True,
    min_samples=100,
)
print(hub.experiment_report(exp["id"]))
print(hub.analytics("order-assistant"))
```

## Errors

| Exception | When |
|---|---|
| `NotFoundError` | prompt/version/experiment/request id not found (404) |
| `ValidationError` | variable validation failed while rendering (422) |
| `APIError` | any other non-2xx, or the server is unreachable |

## Design

See [`docs/learning/07-python-sdk.md`](../../docs/learning/07-python-sdk.md) for the
rationale behind the API shape and the zero-dependency choice.
