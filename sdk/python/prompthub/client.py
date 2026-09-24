"""The PromptHub client and the small value objects it returns.

Transport is stdlib ``urllib`` so the SDK has zero third-party dependencies —
important for a library you drop into an existing app without dragging in a
dependency tree. If you'd rather use ``requests``, the surface is small enough to
swap ``_request`` for it.
"""

from __future__ import annotations

import json
import urllib.error
import urllib.request
from dataclasses import dataclass, field
from typing import Any, Optional

from .errors import APIError, NotFoundError, ValidationError


@dataclass
class RenderedPrompt:
    """The result of rendering a prompt version.

    ``messages`` is the OpenAI-style message list ready to pass to an LLM. The
    ``request_id`` is the handle you pass back to :meth:`PromptHub.log_result`.
    ``experiment_id``/``arm`` are populated only when a running experiment
    assigned this request.
    """

    prompt_name: str
    version_number: int
    system: str
    user: str
    model_config: dict = field(default_factory=dict)
    request_id: str = ""
    experiment_id: Optional[str] = None
    arm: Optional[str] = None

    @property
    def messages(self) -> list:
        """Return chat messages, omitting an empty system prompt."""
        msgs = []
        if self.system:
            msgs.append({"role": "system", "content": self.system})
        msgs.append({"role": "user", "content": self.user})
        return msgs

    @classmethod
    def _from_json(cls, d: dict) -> "RenderedPrompt":
        return cls(
            prompt_name=d.get("prompt_name", ""),
            version_number=d.get("version_number", 0),
            system=d.get("system", ""),
            user=d.get("user", ""),
            model_config=d.get("model_config") or {},
            request_id=d.get("request_id", ""),
            experiment_id=d.get("experiment_id"),
            arm=d.get("arm"),
        )


@dataclass
class Prompt:
    """A lazy handle to a prompt + version selector.

    ``hub.get(...)`` returns one of these; calling :meth:`render` actually hits the
    server. Splitting fetch from render mirrors the plan's ergonomics
    (``hub.get(name, tag=...).render(**vars)``) and lets you reuse one handle for
    many renders.
    """

    _hub: "PromptHub"
    name: str
    tag: Optional[str] = None
    version: Optional[int] = None

    def render(self, *, unit_id: Optional[str] = None, **variables: Any) -> RenderedPrompt:
        """Render this prompt with the given template variables.

        Keyword arguments become template variables. ``unit_id`` (reserved) makes
        experiment assignment sticky for a given user/session.
        """
        return self._hub._render(self.name, self.tag, self.version, unit_id, variables)


class PromptHub:
    """Client for a PromptHub server."""

    def __init__(
        self,
        base_url: str = "http://localhost:8080",
        api_key: Optional[str] = None,
        project: str = "default",
        timeout: float = 10.0,
    ):
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key
        self.project = project
        self.timeout = timeout

    # --- serving path (the two calls most apps use) ---

    def get(self, name: str, *, tag: Optional[str] = None, version: Optional[int] = None) -> Prompt:
        """Return a handle to a prompt. Selection: ``version`` pins exactly,
        else ``tag`` (or the server default) resolves at render time."""
        return Prompt(self, name, tag=tag, version=version)

    def log_result(
        self,
        request_id: str,
        *,
        rating: Optional[int] = None,
        latency_ms: Optional[int] = None,
        cost_usd: Optional[float] = None,
        tokens_used: Optional[int] = None,
        **custom_metrics: Any,
    ) -> None:
        """Report the outcome of a rendered request. Fields are optional and may
        be sent across multiple calls (latency now, the user's rating later)."""
        payload: dict = {"request_id": request_id}
        if rating is not None:
            payload["rating"] = rating
        if latency_ms is not None:
            payload["latency_ms"] = latency_ms
        if cost_usd is not None:
            payload["cost_usd"] = cost_usd
        if tokens_used is not None:
            payload["tokens_used"] = tokens_used
        if custom_metrics:
            payload["custom_metrics"] = custom_metrics
        self._request("POST", "/api/v1/results", payload)

    # --- management surface (used by tooling / the seed script) ---

    def create_prompt(self, name: str, description: str = "", created_by: str = "") -> dict:
        return self._request(
            "POST", "/api/v1/prompts",
            {"name": name, "project": self.project, "description": description, "created_by": created_by},
        )

    def create_version(
        self,
        name: str,
        *,
        system_template: str = "",
        user_template: str = "",
        model_config: Optional[dict] = None,
        variables: Optional[list] = None,
        commit_message: str = "",
        created_by: str = "",
    ) -> dict:
        return self._request(
            "POST", f"/api/v1/prompts/{name}/versions",
            {
                "system_template": system_template,
                "user_template": user_template,
                "model_config": model_config or {},
                "variables": variables or [],
                "commit_message": commit_message,
                "created_by": created_by,
            },
        )

    def set_tag(self, name: str, tag: str, version_number: int, updated_by: str = "") -> dict:
        """Point ``tag`` at ``version_number``. Deploy and rollback are the same call."""
        return self._request(
            "PUT", f"/api/v1/prompts/{name}/tags/{tag}",
            {"version_number": version_number, "updated_by": updated_by},
        )

    def rollback(self, name: str, tag: str, version_number: int, updated_by: str = "") -> dict:
        """Alias for :meth:`set_tag` that reads intent-first at call sites."""
        return self.set_tag(name, tag, version_number, updated_by)

    def get_prompt(self, name: str) -> dict:
        return self._request("GET", f"/api/v1/prompts/{name}")

    def list_prompts(self) -> list:
        return self._request("GET", "/api/v1/prompts").get("prompts", [])

    def diff(self, name: str, from_version: int, to_version: int, mode: str = "line") -> dict:
        return self._request(
            "GET", f"/api/v1/prompts/{name}/diff?from={from_version}&to={to_version}&mode={mode}"
        )

    def create_experiment(
        self,
        prompt: str,
        control_version: int,
        variant_version: int,
        *,
        name: str = "",
        traffic_split: float = 0.5,
        auto_promote: bool = False,
        min_samples: int = 100,
    ) -> dict:
        return self._request(
            "POST", "/api/v1/experiments",
            {
                "prompt": prompt,
                "name": name,
                "control_version": control_version,
                "variant_version": variant_version,
                "traffic_split": traffic_split,
                "auto_promote": auto_promote,
                "min_samples": min_samples,
            },
        )

    def experiment_report(self, experiment_id: str) -> dict:
        return self._request("GET", f"/api/v1/experiments/{experiment_id}/report")

    def analytics(self, name: str) -> dict:
        """Per-version render analytics for a prompt (latency/cost/satisfaction)."""
        return self._request("GET", f"/api/v1/prompts/{name}/analytics")

    # --- internals ---

    def _render(self, name, tag, version, unit_id, variables) -> RenderedPrompt:
        payload: dict = {"variables": variables or {}}
        if tag:
            payload["tag"] = tag
        if version:
            payload["version"] = version
        if unit_id:
            payload["unit_id"] = unit_id
        data = self._request("POST", f"/api/v1/prompts/{name}/render", payload)
        return RenderedPrompt._from_json(data)

    def _request(self, method: str, path: str, body: Optional[dict] = None):
        url = self.base_url + path
        # A project query param applies to prompt-scoped routes; harmless elsewhere.
        if self.project and self.project != "default" and "project=" not in path:
            sep = "&" if "?" in path else "?"
            url = f"{url}{sep}project={self.project}"

        data = json.dumps(body).encode("utf-8") if body is not None else None
        req = urllib.request.Request(url, data=data, method=method)
        req.add_header("Content-Type", "application/json")
        if self.api_key:
            req.add_header("Authorization", f"Bearer {self.api_key}")

        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                raw = resp.read()
                return json.loads(raw) if raw else {}
        except urllib.error.HTTPError as e:
            raw = e.read().decode("utf-8", "replace")
            message = self._extract_error(raw)
            if e.code == 404:
                raise NotFoundError(message) from None
            if e.code == 422:
                raise ValidationError(message) from None
            raise APIError(e.code, message, raw) from None
        except urllib.error.URLError as e:
            raise APIError(0, f"could not reach {url}: {e.reason}") from None

    @staticmethod
    def _extract_error(raw: str) -> str:
        try:
            return json.loads(raw).get("error", raw)
        except (ValueError, AttributeError):
            return raw or "unknown error"
