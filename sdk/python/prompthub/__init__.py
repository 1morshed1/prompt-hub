"""PromptHub Python SDK.

One-liner integration for using version-controlled, A/B-tested prompts from your
LLM application:

    from prompthub import PromptHub

    hub = PromptHub(base_url="http://localhost:8080", project="customer-support")

    # Fetch + render the production version (server-side templating).
    rendered = hub.get("order-assistant", tag="production").render(
        role="support agent", topic="refunds", query="Where is my order?", language="en",
    )
    messages = rendered.messages            # ready for the LLM
    model = rendered.model_config.get("model")

    # ... call your LLM with `messages` ...

    # Report the outcome so PromptHub can track performance / decide experiments.
    hub.log_result(rendered.request_id, rating=1, latency_ms=1200, cost_usd=0.0012)

The SDK is pure standard library (no third-party dependencies), so it installs
and runs anywhere. See docs/learning/07-python-sdk.md for the design rationale.
"""

from .client import PromptHub, Prompt, RenderedPrompt
from .errors import PromptHubError, NotFoundError, ValidationError, APIError

__all__ = [
    "PromptHub",
    "Prompt",
    "RenderedPrompt",
    "PromptHubError",
    "NotFoundError",
    "ValidationError",
    "APIError",
]

__version__ = "0.1.0"
