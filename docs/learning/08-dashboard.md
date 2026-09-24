# 08 — The dashboard

The dashboard is a deliberately **buildless** single-page app: three static files
(`web/index.html`, `web/styles.css`, `web/app.js`) served directly by the Go
binary. No npm, no bundler, no framework. If you can read `fetch()` and template
literals, you can read the whole UI. Code: `web/`.

## Why buildless

The dashboard is a thin *read/write view over the JSON API* — it has no state the
API doesn't already own. A React/Vite toolchain would add a build step,
`node_modules`, and a deploy artifact for no real benefit at this scale. Keeping it
to plain files means:

- the Go server serves it directly (`http.FileServer` at `/`),
- there's nothing to build or install to run it,
- every line of behavior is visible, which suits a learning repo.

This is the reasoning in [ADR-0001](../decisions/0001-language-and-stack.md).

## How it's structured (~350 lines of `app.js`)

Three small ideas do all the work:

1. **`api(path, opts)`** — a `fetch` wrapper that prefixes `/api/v1`, parses JSON,
   and throws a readable `Error` on non-2xx. Every view calls this.

2. **`h(tag, attrs, ...children)`** — a ~15-line hyperscript helper. It creates an
   element, wires attributes (function values like `onclick` become listeners),
   and appends children. This is "just enough" of what JSX gives you, without a
   compiler. Views build DOM by composing `h(...)` calls.

3. **A hash router** — an array of `[regex, viewFunction]` pairs. On `hashchange`,
   the first matching route's view runs, fetches its data, and renders. URLs look
   like `#/`, `#/prompt/order-assistant`, `#/experiments`, `#/experiment/{id}`.

Everything else is view functions returning DOM.

## The views

- **Prompts list** (`#/`) — cards linking to each prompt, plus an inline "new
  prompt" form.
- **Prompt detail** (`#/prompt/{name}`) — the richest view:
  - **Tags** shown as `production → v3` badges.
  - **Version history timeline**: each version card shows its number, the tags
    pointing at it, its commit message, a collapsible view of the templates/model
    config, and its **per-version analytics** (renders, avg rating, avg latency,
    tokens) pulled from `/prompts/{name}/analytics`.
  - **Set tag…** on any version — the deploy/rollback action, one prompt() away.
  - **Compare versions**: pick two versions and a mode (line/token); the rendered
    diff colors insertions green and deletions red, using the API's diff segments.
  - **New version** form with fields for both templates, variables (JSON), and
    model config.
- **Experiments list** (`#/experiments`) — cards with split, auto-promote, and a
  status badge (running/promoted/stopped).
- **Experiment report** (`#/experiment/{id}`) — the payoff view:
  - a **winner banner**,
  - the plain-English **recommendation**,
  - a **side-by-side** control/variant comparison with up-rate bars,
  - the **statistical significance** block (χ² for ratings, Mann-Whitney U for
    latency, with p-values and a significant/not-significant verdict),
  - the **auto-promotion** decision.

## Theming

`styles.css` uses CSS custom properties for colors and a
`@media (prefers-color-scheme: dark)` block, so the dashboard follows the OS
light/dark setting with no JS. All colors are tokens on `:root`, redefined once
for dark mode — the same discipline you'd use in a design system, just hand-rolled.

## How it was verified

The four views were rendered in headless Chromium and screenshotted end-to-end
against a seeded server (`scripts/seed.py`), confirming zero console errors and
that real data flows through: the version timeline shows per-version rating/latency,
and the experiment report shows the χ²/Mann-Whitney verdicts and the promoted
winner. See the commit that added this doc.

## Run it

```bash
# server running with WEB_DIR pointing at web/ (the default is ../web)
open http://localhost:8080/
```
