"""Exception types raised by the SDK.

We map the API's HTTP status codes to distinct exception classes so callers can
handle, say, a validation failure differently from a missing prompt, without
inspecting status codes themselves.
"""


class PromptHubError(Exception):
    """Base class for every error the SDK raises."""


class NotFoundError(PromptHubError):
    """A prompt, version, or experiment was not found (HTTP 404)."""


class ValidationError(PromptHubError):
    """Variable validation failed while rendering (HTTP 422).

    The message lists every problem (missing required variable, wrong type).
    """


class APIError(PromptHubError):
    """Any other non-2xx response from the API.

    Attributes:
        status: the HTTP status code.
        body: the raw response body (usually ``{"error": "..."}``).
    """

    def __init__(self, status, message, body=None):
        super().__init__(f"API error {status}: {message}")
        self.status = status
        self.body = body
