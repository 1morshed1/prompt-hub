package template

import (
	"fmt"
	"strings"
)

// eval walks a parsed node tree against a context and returns the rendered text.
// It's a method on Engine because rendering needs the IncludeResolver.
func (e *Engine) eval(nodes []Node, ctx map[string]any) (string, error) {
	var b strings.Builder
	if err := e.evalInto(&b, nodes, ctx, 0); err != nil {
		return "", err
	}
	return b.String(), nil
}

// evalInto is the recursive worker. depth counts include-nesting to stop runaway
// recursion or include cycles.
func (e *Engine) evalInto(b *strings.Builder, nodes []Node, ctx map[string]any, depth int) error {
	for _, n := range nodes {
		switch node := n.(type) {
		case TextNode:
			b.WriteString(node.Text)

		case OutputNode:
			v, err := node.Expr.eval(ctx)
			if err != nil {
				return err
			}
			b.WriteString(stringify(v))

		case IfNode:
			if err := e.evalIf(b, node, ctx, depth); err != nil {
				return err
			}

		case ForNode:
			if err := e.evalFor(b, node, ctx, depth); err != nil {
				return err
			}

		case IncludeNode:
			if err := e.evalInclude(b, node, ctx, depth); err != nil {
				return err
			}

		default:
			return fmt.Errorf("internal error: unknown node type %T", n)
		}
	}
	return nil
}

func (e *Engine) evalIf(b *strings.Builder, node IfNode, ctx map[string]any, depth int) error {
	for _, br := range node.Branches {
		v, err := br.Cond.eval(ctx)
		if err != nil {
			return err
		}
		if truthy(v) {
			return e.evalInto(b, br.Body, ctx, depth)
		}
	}
	return e.evalInto(b, node.Else, ctx, depth)
}

func (e *Engine) evalFor(b *strings.Builder, node ForNode, ctx map[string]any, depth int) error {
	iter, err := node.Iter.eval(ctx)
	if err != nil {
		return err
	}
	list, err := asList(iter)
	if err != nil {
		return fmt.Errorf("in {%% for %s in ... %%}: %w", node.Var, err)
	}
	for i, item := range list {
		// Child scope: copy the parent context and bind the loop variable plus a
		// `loop` helper (index/first/last) that real templates lean on.
		scope := childScope(ctx)
		scope[node.Var] = item
		scope["loop"] = map[string]any{
			"index":  float64(i),     // 0-based
			"index1": float64(i + 1), // 1-based
			"first":  i == 0,
			"last":   i == len(list)-1,
		}
		if err := e.evalInto(b, node.Body, scope, depth); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) evalInclude(b *strings.Builder, node IncludeNode, ctx map[string]any, depth int) error {
	if depth >= e.maxIncludes {
		return fmt.Errorf("include depth limit (%d) exceeded — possible include cycle at %q",
			e.maxIncludes, node.Name)
	}
	src, err := e.resolver.Resolve(node.Name)
	if err != nil {
		return fmt.Errorf("include %q: %w", node.Name, err)
	}
	sub, err := Parse(src)
	if err != nil {
		return fmt.Errorf("parsing included template %q: %w", node.Name, err)
	}
	// Includes render in the same context (they can see the caller's variables),
	// at one deeper include level.
	return e.evalInto(b, sub, ctx, depth+1)
}

// childScope makes a shallow copy of a context so loop-variable bindings don't
// leak back into the parent scope.
func childScope(parent map[string]any) map[string]any {
	c := make(map[string]any, len(parent)+2)
	for k, v := range parent {
		c[k] = v
	}
	return c
}
