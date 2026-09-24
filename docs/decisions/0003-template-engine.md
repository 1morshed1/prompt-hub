# ADR-0003 — Hand-written template engine

**Status:** Accepted
**Date:** 2026-09-24

## Context

Real production prompts are not strings. They contain conditional sections ("if the user's
language is Bengali, prepend the Bengali safety preamble"), loops (over retrieved documents),
and shared includes (a safety preamble reused across many prompts). The plan specifies this
syntax explicitly:

```
{{ query }}                              variable substitution
{% if language == "bn" %}...{% endif %}  conditionals
{% for doc in documents %}...{% endfor %} loops
{% include "shared/safety-preamble" %}    includes
```

Plus **type validation**: a version declares its variables (`[{name:"role", type:"string"}]`)
and rendering must fail loudly if a caller passes the wrong type or omits a required variable.

## Options considered

1. **Go's `text/template`.** Rejected: its syntax (`{{if}}`, `{{range}}`, pipelines) is not
   the syntax in the spec, its error messages are opaque to prompt authors, and bending it to
   support our `{% include %}` from the database would be more work than writing our own.
2. **A third-party Jinja-like library.** Rejected: adds a dependency for the single most
   educational component in the codebase, and we'd still have to bolt on type validation and
   DB-backed includes.
3. **Write our own lexer → parser → evaluator.** Chosen.

## Decision

Implement a small template engine as three classic stages:

1. **Lexer** turns the template string into tokens (`TEXT`, `VAR`, `TAG`).
2. **Parser** builds an AST of nodes (`TextNode`, `VarNode`, `IfNode`, `ForNode`,
   `IncludeNode`). This is where `{% if %}…{% endif %}` nesting is validated structurally.
3. **Evaluator** walks the AST with a variable context and produces the final string.
   Includes are resolved through a pluggable `IncludeResolver` so the same engine works in
   tests (map-backed) and in production (database-backed).

Type validation is a separate, explicit pass over the declared variable schema, run before
evaluation, so callers get one clear error listing everything wrong rather than a crash
mid-render.

## Consequences

- ~400 lines of well-tested Go, but it's the part of the codebase most worth reading, and it
  has zero external dependencies.
- The three-stage structure is textbook, so it's easy to explain and extend (adding
  `{% elif %}` or filters later is localized to parser + evaluator).
- `IncludeResolver` as an interface keeps the engine pure and unit-testable without a database.

See `docs/learning/04-template-engine.md` for the full build-up with examples.
