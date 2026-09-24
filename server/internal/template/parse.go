package template

import (
	"fmt"
	"strings"
)

// Parse lexes src and builds the node tree. This stage owns block structure: it's
// where {% if %}...{% endif %} and {% for %}...{% endfor %} nesting is validated.
// A mismatched or unclosed block is a parse error, caught here rather than at
// render time.
func Parse(src string) ([]Node, error) {
	chunks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{chunks: chunks}
	nodes, term, err := p.parseNodes()
	if err != nil {
		return nil, err
	}
	if term != "" {
		return nil, fmt.Errorf("unexpected {%% %s %%} with no matching opening tag", term)
	}
	return nodes, nil
}

type parser struct {
	chunks []chunk
	pos    int
}

// terminators are the tag keywords that close or continue a block. parseNodes
// stops (without consuming) when it hits one, returning which it was so the
// caller (an if/for parser) can decide what to do.
var terminators = map[string]bool{"endif": true, "endfor": true, "elif": true, "else": true}

// parseNodes parses a run of nodes until EOF or a terminator tag. It returns the
// nodes, the terminator keyword ("" at EOF), and any error. The terminator chunk
// is consumed.
func (p *parser) parseNodes() ([]Node, string, error) {
	var nodes []Node
	for p.pos < len(p.chunks) {
		c := p.chunks[p.pos]
		switch c.kind {
		case chunkText:
			p.pos++
			if c.body != "" {
				nodes = append(nodes, TextNode{Text: c.body})
			}
		case chunkOutput:
			p.pos++
			e, err := parseExpr(c.body)
			if err != nil {
				return nil, "", fmt.Errorf("in {{ %s }}: %w", c.body, err)
			}
			nodes = append(nodes, OutputNode{Expr: e})
		case chunkTag:
			keyword := firstWord(c.body)
			if terminators[keyword] {
				p.pos++ // consume the terminator
				return nodes, keyword, nil
			}
			node, err := p.parseTag(c)
			if err != nil {
				return nil, "", err
			}
			nodes = append(nodes, node)
		}
	}
	return nodes, "", nil
}

// parseTag dispatches an opening statement tag (if / for / include).
func (p *parser) parseTag(c chunk) (Node, error) {
	keyword := firstWord(c.body)
	switch keyword {
	case "if":
		return p.parseIf(c)
	case "for":
		return p.parseFor(c)
	case "include":
		p.pos++
		return parseInclude(c.body)
	default:
		return nil, fmt.Errorf("unknown tag {%% %s %%}", c.body)
	}
}

// parseIf handles if / elif* / else? / endif. It consumes the opening tag, then
// parses branch bodies, following elif/else terminators until endif.
func (p *parser) parseIf(c chunk) (Node, error) {
	p.pos++ // consume the opening {% if ... %}
	cond, err := parseExpr(strings.TrimSpace(strings.TrimPrefix(c.body, "if")))
	if err != nil {
		return nil, fmt.Errorf("in {%% if %s %%}: %w", c.body, err)
	}
	ifNode := IfNode{}
	body, term, err := p.parseNodes()
	if err != nil {
		return nil, err
	}
	ifNode.Branches = append(ifNode.Branches, Branch{Cond: cond, Body: body})

	for term == "elif" {
		// The elif chunk was consumed by parseNodes; recover its expression from
		// the chunk just before pos.
		elifChunk := p.chunks[p.pos-1]
		elifCond, err := parseExpr(strings.TrimSpace(strings.TrimPrefix(elifChunk.body, "elif")))
		if err != nil {
			return nil, fmt.Errorf("in {%% elif %%}: %w", err)
		}
		elifBody, t, err := p.parseNodes()
		if err != nil {
			return nil, err
		}
		ifNode.Branches = append(ifNode.Branches, Branch{Cond: elifCond, Body: elifBody})
		term = t
	}
	if term == "else" {
		elseBody, t, err := p.parseNodes()
		if err != nil {
			return nil, err
		}
		ifNode.Else = elseBody
		term = t
	}
	if term != "endif" {
		return nil, fmt.Errorf("unclosed {%% if %%} (expected {%% endif %%}, got %q)", term)
	}
	return ifNode, nil
}

// parseFor handles {% for x in expr %}...{% endfor %}.
func (p *parser) parseFor(c chunk) (Node, error) {
	p.pos++ // consume opening tag
	varName, iterExpr, err := parseForHeader(c.body)
	if err != nil {
		return nil, err
	}
	body, term, err := p.parseNodes()
	if err != nil {
		return nil, err
	}
	if term != "endfor" {
		return nil, fmt.Errorf("unclosed {%% for %%} (expected {%% endfor %%}, got %q)", term)
	}
	return ForNode{Var: varName, Iter: iterExpr, Body: body}, nil
}

// parseForHeader parses "for <ident> in <expr>".
func parseForHeader(body string) (string, Expr, error) {
	rest := strings.TrimSpace(strings.TrimPrefix(body, "for"))
	// Split on the first " in " keyword.
	idx := strings.Index(rest, " in ")
	if idx < 0 {
		return "", nil, fmt.Errorf("malformed for tag %q (expected: for x in items)", body)
	}
	varName := strings.TrimSpace(rest[:idx])
	iterSrc := strings.TrimSpace(rest[idx+len(" in "):])
	if varName == "" || iterSrc == "" {
		return "", nil, fmt.Errorf("malformed for tag %q", body)
	}
	iter, err := parseExpr(iterSrc)
	if err != nil {
		return "", nil, fmt.Errorf("in for-loop iterable %q: %w", iterSrc, err)
	}
	return varName, iter, nil
}

// parseInclude parses `include "name"`.
func parseInclude(body string) (Node, error) {
	rest := strings.TrimSpace(strings.TrimPrefix(body, "include"))
	if len(rest) < 2 || (rest[0] != '"' && rest[0] != '\'') {
		return nil, fmt.Errorf("include name must be a quoted string, got %q", body)
	}
	quote := rest[0]
	end := strings.IndexByte(rest[1:], quote)
	if end < 0 {
		return nil, fmt.Errorf("unterminated include name in %q", body)
	}
	return IncludeNode{Name: rest[1 : 1+end]}, nil
}

func firstWord(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " \t\n"); i >= 0 {
		return s[:i]
	}
	return s
}
