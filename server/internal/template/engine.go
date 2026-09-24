// Package template implements PromptHub's prompt template engine.
//
// It supports the syntax the product spec requires:
//
//	{{ query }}                                 variable substitution (dotted: {{ user.name }})
//	{% if language == "bn" %}...{% endif %}     conditionals (elif / else supported)
//	{% for doc in documents %}...{% endfor %}   loops
//	{% include "shared/safety" %}               includes (resolved via IncludeResolver)
//
// The implementation is the classic three-stage compiler pipeline, each stage in
// its own file so you can read them in order:
//
//	lex.go     source string  ->  []chunk        (TEXT / OUTPUT / TAG)
//	parse.go   []chunk        ->  []Node (an AST) (validates block nesting)
//	expr.go    condition text ->  expr AST + eval (the {% if %} / for-in language)
//	eval.go    AST + context  ->  rendered string
//	validate.go variables + input -> typed, validated context (or a clear error)
//
// See docs/learning/04-template-engine.md for the full walkthrough.
package template

import "fmt"

// IncludeResolver fetches the source of an included template by name. Making this
// an interface keeps the engine pure: tests use a map-backed resolver, production
// uses a database-backed one, and the engine itself never knows the difference.
type IncludeResolver interface {
	Resolve(name string) (string, error)
}

// ResolverFunc adapts a plain function to the IncludeResolver interface.
type ResolverFunc func(name string) (string, error)

func (f ResolverFunc) Resolve(name string) (string, error) { return f(name) }

// noIncludes is the default resolver: it errors if a template uses {% include %}
// but no resolver was configured, rather than silently rendering nothing.
type noIncludes struct{}

func (noIncludes) Resolve(name string) (string, error) {
	return "", fmt.Errorf("template uses {%% include %q %%} but no include resolver is configured", name)
}

// Engine renders templates. It's cheap to construct and safe for concurrent use
// (it holds no per-render state).
type Engine struct {
	resolver    IncludeResolver
	maxIncludes int // guards against include cycles / runaway recursion
}

// Option configures an Engine.
type Option func(*Engine)

// WithIncludeResolver sets how {% include %} names are resolved.
func WithIncludeResolver(r IncludeResolver) Option {
	return func(e *Engine) { e.resolver = r }
}

// New builds an Engine.
func New(opts ...Option) *Engine {
	e := &Engine{resolver: noIncludes{}, maxIncludes: 50}
	for _, o := range opts {
		o(e)
	}
	return e
}

// Render parses src and evaluates it against ctx, returning the final string.
// Parsing and evaluation are separate so a caller could cache the parsed AST; the
// convenience path here does both.
func (e *Engine) Render(src string, ctx map[string]any) (string, error) {
	nodes, err := Parse(src)
	if err != nil {
		return "", err
	}
	return e.eval(nodes, ctx)
}
