package template

import (
	"strings"
	"testing"

	"github.com/1morshed1/prompt-hub/server/internal/model"
)

func render(t *testing.T, src string, ctx map[string]any, opts ...Option) string {
	t.Helper()
	out, err := New(opts...).Render(src, ctx)
	if err != nil {
		t.Fatalf("render(%q): %v", src, err)
	}
	return out
}

func TestVariableSubstitution(t *testing.T) {
	got := render(t, "Hello {{ name }}!", map[string]any{"name": "Ada"})
	if got != "Hello Ada!" {
		t.Errorf("got %q", got)
	}
}

func TestDottedPath(t *testing.T) {
	ctx := map[string]any{"user": map[string]any{"name": "Bo"}}
	if got := render(t, "{{ user.name }}", ctx); got != "Bo" {
		t.Errorf("got %q", got)
	}
	// Missing intermediate renders empty, not an error.
	if got := render(t, "[{{ user.missing.deep }}]", ctx); got != "[]" {
		t.Errorf("missing path should be empty, got %q", got)
	}
}

func TestNumberFormatting(t *testing.T) {
	// JSON numbers arrive as float64; whole numbers should print without ".0".
	if got := render(t, "{{ n }}", map[string]any{"n": float64(3)}); got != "3" {
		t.Errorf("got %q, want 3", got)
	}
	if got := render(t, "{{ n }}", map[string]any{"n": 3.5}); got != "3.5" {
		t.Errorf("got %q, want 3.5", got)
	}
}

func TestConditionalEquality(t *testing.T) {
	src := `{% if language == "bn" %}Bengali{% else %}English{% endif %}`
	if got := render(t, src, map[string]any{"language": "bn"}); got != "Bengali" {
		t.Errorf("got %q", got)
	}
	if got := render(t, src, map[string]any{"language": "en"}); got != "English" {
		t.Errorf("got %q", got)
	}
}

func TestElifChain(t *testing.T) {
	src := `{% if n > 90 %}A{% elif n > 80 %}B{% elif n > 70 %}C{% else %}F{% endif %}`
	cases := map[float64]string{95: "A", 85: "B", 75: "C", 50: "F"}
	for n, want := range cases {
		if got := render(t, src, map[string]any{"n": n}); got != want {
			t.Errorf("n=%v got %q want %q", n, got, want)
		}
	}
}

func TestLogicalOperators(t *testing.T) {
	src := `{% if premium and count > 2 %}yes{% else %}no{% endif %}`
	if got := render(t, src, map[string]any{"premium": true, "count": float64(3)}); got != "yes" {
		t.Errorf("got %q", got)
	}
	if got := render(t, src, map[string]any{"premium": true, "count": float64(1)}); got != "no" {
		t.Errorf("got %q", got)
	}
	if got := render(t, `{% if not blocked %}ok{% endif %}`, map[string]any{"blocked": false}); got != "ok" {
		t.Errorf("not-operator got %q", got)
	}
}

func TestLoop(t *testing.T) {
	ctx := map[string]any{"docs": []any{"a", "b", "c"}}
	got := render(t, `{% for d in docs %}{{ d }}{% endfor %}`, ctx)
	if got != "abc" {
		t.Errorf("got %q", got)
	}
}

func TestLoopHelpers(t *testing.T) {
	ctx := map[string]any{"items": []any{"x", "y", "z"}}
	src := `{% for it in items %}{{ loop.index1 }}:{{ it }}{% if not loop.last %},{% endif %}{% endfor %}`
	got := render(t, src, ctx)
	if got != "1:x,2:y,3:z" {
		t.Errorf("got %q", got)
	}
}

func TestLoopOverObjects(t *testing.T) {
	ctx := map[string]any{"docs": []any{
		map[string]any{"title": "Refund policy"},
		map[string]any{"title": "Shipping"},
	}}
	src := `{% for doc in docs %}- {{ doc.title }}
{% endfor %}`
	got := render(t, src, ctx)
	want := "- Refund policy\n- Shipping\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestNestedBlocks(t *testing.T) {
	ctx := map[string]any{
		"users": []any{
			map[string]any{"name": "A", "admin": true},
			map[string]any{"name": "B", "admin": false},
		},
	}
	src := `{% for u in users %}{{ u.name }}{% if u.admin %}(admin){% endif %} {% endfor %}`
	got := render(t, src, ctx)
	if got != "A(admin) B " {
		t.Errorf("got %q", got)
	}
}

func TestInclude(t *testing.T) {
	resolver := ResolverFunc(func(name string) (string, error) {
		if name == "shared/safety" {
			return "SAFETY: be kind.", nil
		}
		return "", errNotFound(name)
	})
	src := `{% include "shared/safety" %}
User: {{ q }}`
	got := render(t, src, map[string]any{"q": "hi"}, WithIncludeResolver(resolver))
	if !strings.Contains(got, "SAFETY: be kind.") || !strings.Contains(got, "User: hi") {
		t.Errorf("include failed: %q", got)
	}
}

func TestIncludeSeesContext(t *testing.T) {
	resolver := ResolverFunc(func(name string) (string, error) {
		return "Hello {{ name }} from include", nil
	})
	got := render(t, `{% include "greeting" %}`, map[string]any{"name": "Zed"}, WithIncludeResolver(resolver))
	if got != "Hello Zed from include" {
		t.Errorf("got %q", got)
	}
}

func TestUnclosedBlockErrors(t *testing.T) {
	if _, err := New().Render(`{% if x %}no end`, nil); err == nil {
		t.Error("expected error for unclosed if")
	}
	if _, err := New().Render(`{% for x in xs %}`, nil); err == nil {
		t.Error("expected error for unclosed for")
	}
	if _, err := New().Render(`{{ unclosed`, nil); err == nil {
		t.Error("expected error for unclosed output")
	}
	if _, err := New().Render(`{% endif %}`, nil); err == nil {
		t.Error("expected error for dangling endif")
	}
}

func TestValidationRequired(t *testing.T) {
	vars := []model.Variable{
		{Name: "role", Type: "string", Required: true},
		{Name: "count", Type: "number", Required: false, Default: float64(1)},
	}
	// Missing required -> error listing it.
	if _, err := BuildContext(vars, map[string]any{}); err == nil {
		t.Error("expected required-field error")
	} else if !strings.Contains(err.Error(), "role") {
		t.Errorf("error should mention role: %v", err)
	}

	// Default filled for optional.
	ctx, err := BuildContext(vars, map[string]any{"role": "agent"})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if ctx["count"] != float64(1) {
		t.Errorf("default not applied: %v", ctx["count"])
	}
}

func TestValidationTypeMismatch(t *testing.T) {
	vars := []model.Variable{{Name: "count", Type: "number", Required: true}}
	_, err := BuildContext(vars, map[string]any{"count": "not a number"})
	if err == nil || !strings.Contains(err.Error(), "count") {
		t.Errorf("expected type error mentioning count, got %v", err)
	}
}

// errNotFound is a tiny helper so the include test reads cleanly.
func errNotFound(name string) error { return &notFoundErr{name} }

type notFoundErr struct{ name string }

func (e *notFoundErr) Error() string { return "not found: " + e.name }
