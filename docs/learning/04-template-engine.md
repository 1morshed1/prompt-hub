# 04 — The template engine

This is the most instructive subsystem in the repo. It's a small but complete **compiler
pipeline**: text goes in, a rendered prompt comes out, through the same three stages every
language implementation uses. Code: `server/internal/template/`.

## Why prompts need a language at all

Real production prompts aren't strings. They look like this:

```
You are a {{ role }} assistant.
{% if language == "bn" %}
Reply in Bengali. Be culturally appropriate.
{% endif %}
Here are the relevant documents:
{% for doc in documents %}
- {{ doc.title }}: {{ doc.snippet }}
{% endfor %}
{% include "shared/safety-preamble" %}
Answer this question: {{ query }}
```

Variables, conditionals, loops, includes. That's a template language, so we built one.

## The pipeline

```
 source string
      │  lex.go        (Stage 1: lexer)
      ▼
 []chunk   ── TEXT | OUTPUT {{…}} | TAG {%…%}
      │  parse.go      (Stage 2: parser)
      ▼
 []Node    ── an AST: TextNode, OutputNode, IfNode, ForNode, IncludeNode
      │  eval.go       (Stage 3: evaluator, walks the AST with a context)
      ▼
 rendered string
```

Two support files round it out:
- `expr.go` — a *second, nested* mini-language for the stuff inside `{{ }}` and `{% if %}`.
- `values.go` — runtime value semantics (truthiness, equality, number formatting).
- `validate.go` — the type-checking pass over declared variables.

### Stage 1 — Lexer (`lex.go`)

The lexer does one dumb, linear job: scan the source and cut it into `chunk`s of three kinds —
literal `TEXT`, an `OUTPUT` (`{{ … }}`), or a `TAG` (`{% … %}`). It finds the next `{{` or
`{%`, emits the text before it, then emits the delimited piece with its delimiters stripped.

Crucially, **the lexer knows nothing about if/for nesting.** It doesn't know that `{% endif %}`
closes anything. It just knows "here is a tag." Keeping the lexer ignorant of structure is what
keeps it 60 lines. Structure is the parser's problem.

### Stage 2 — Parser (`parse.go`)

The parser turns the flat `[]chunk` into a **tree** (`[]Node`). This is where block structure
is enforced. The core is `parseNodes`, which consumes chunks until it hits a *terminator* tag
(`endif`, `endfor`, `elif`, `else`) and returns which one stopped it. The block parsers use
that:

- `parseIf` reads the `if` body, and if the terminator was `elif` or `else` it keeps going,
  assembling a list of `Branch`es and an optional `Else`, until it sees `endif`.
- `parseFor` reads the loop body until `endfor`.

An `{% if %}` with no `{% endif %}`, or a stray `{% endif %}` with no opening, is a **parse
error** — caught here, before we ever try to render. (Test: `TestUnclosedBlockErrors`.)

### The nested expression language (`expr.go`)

Inside `{{ … }}` and `{% if … %}` there's a whole second language: `language == "bn"`,
`premium and count > 2`, `not blocked`, `user.name`. `expr.go` is a self-contained
**recursive-descent parser** for it, with this precedence ladder (loosest binding first):

```
or  →  and  →  not  →  comparison  →  primary
```

`primary` is a literal (`"bn"`, `42`, `true`), a parenthesized sub-expression, or a **dotted
path** (`user.name`). Each precedence level is one function that calls the next level down —
the textbook shape of a recursive-descent parser, and the clearest way to see how operator
precedence actually works. Evaluation is a method on each node type; logical `and`/`or`
short-circuit.

Why so small? Because prompt conditions are simple. The grammar covers every real case
(`language == "bn"`, `tier == "gold" and credits > 0`) without becoming a scripting language
you'd have to sandbox.

### Stage 3 — Evaluator (`eval.go`)

`evalInto` walks the node tree and appends to a `strings.Builder`:

- `TextNode` → write it.
- `OutputNode` → evaluate the expression, `stringify` the result, write it.
- `IfNode` → evaluate each branch's condition in order; render the first truthy one, else the
  `Else`.
- `ForNode` → coerce the iterable to a list; for each element create a **child scope** (a copy
  of the context) binding the loop variable and a `loop` helper (`loop.index`, `loop.first`,
  `loop.last`), then render the body. The copy is what stops loop variables leaking out.
- `IncludeNode` → ask the `IncludeResolver` for the named source, parse it, render it **in the
  same context** (so includes see the caller's variables), one nesting level deeper. A depth
  counter (`maxIncludes`) stops include cycles from looping forever.

### Value semantics (`values.go`)

Small decisions that make the engine feel right:

- **Truthiness**: `nil`, `false`, `0`, `""`, empty list/map are falsy; everything else truthy.
- **Numbers**: JSON decodes every number as `float64`, so `3` arrives as `3.0`. `stringify`
  prints whole floats without the `.0` — users expect `{{ count }}` → `3`, not `3.0`.
- **Equality** works across int/float, and `<,>,<=,>=` require numbers (comparing strings with
  `>` is almost always a bug, so it errors loudly).

### The validation pass (`validate.go`)

Separate from rendering, `BuildContext(vars, input)` checks caller input against the version's
declared variable schema **before** evaluation:

- missing `required` variable → error,
- omitted optional variable → its declared default is filled in,
- wrong type → error.

It collects **all** problems into one `ValidationError` rather than failing on the first, so a
caller sees "role is required; count must be a number" in one shot. This is the difference
between a tool engineers trust and one that fails halfway through a render with a cryptic
message. (Tests: `TestValidationRequired`, `TestValidationTypeMismatch`.)

## Design decision recap

Why hand-write all this instead of using Go's `text/template` or a Jinja library? Because the
spec needs specific `{% %}` syntax, DB-backed includes, and type validation — and because this
pipeline is the single most valuable thing in the repo to be able to read and explain. The full
argument is in [ADR-0003](../decisions/0003-template-engine.md).

## Try it (in Go)

```go
eng := template.New()
out, _ := eng.Render(
    `{% if vip %}Dear {{ name }}{% else %}Hi{% endif %}`,
    map[string]any{"vip": true, "name": "Ada"},
)
// out == "Dear Ada"
```

Or read `engine_test.go` — every feature above has a focused test you can run with
`go test ./internal/template/`.
