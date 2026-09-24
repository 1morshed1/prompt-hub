package template

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// This file implements the small expression language used inside {{ }} outputs
// and {% if %}/{% elif %} conditions. It's a self-contained recursive-descent
// parser + evaluator with this grammar (lowest precedence first):
//
//	or         := and ( "or" and )*
//	and        := not ( "and" not )*
//	not        := "not" not | comparison
//	comparison := primary ( ("==" | "!=" | "<" | ">" | "<=" | ">=") primary )?
//	primary    := "(" or ")" | number | string | "true" | "false" | path
//	path       := IDENT ( "." IDENT )*
//
// Keeping the grammar tiny is deliberate: it covers every condition real prompts
// need ( language == "bn", not premium, count > 0 and tier == "gold" ) without
// turning into a general-purpose scripting language.

// Expr is an evaluatable expression node.
type Expr interface {
	eval(ctx map[string]any) (any, error)
}

// --- expression tokens ---

type exprTokKind int

const (
	etEOF exprTokKind = iota
	etIdent
	etNumber
	etString
	etOp     // == != < > <= >=
	etLParen // (
	etRParen // )
	etDot    // .
	etKeyword
)

type exprTok struct {
	kind exprTokKind
	val  string
}

// lexExpr tokenizes an expression string.
func lexExpr(s string) ([]exprTok, error) {
	var toks []exprTok
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '(':
			toks = append(toks, exprTok{etLParen, "("})
			i++
		case c == ')':
			toks = append(toks, exprTok{etRParen, ")"})
			i++
		case c == '.':
			toks = append(toks, exprTok{etDot, "."})
			i++
		case c == '"' || c == '\'':
			str, n, err := lexString(s[i:], c)
			if err != nil {
				return nil, err
			}
			toks = append(toks, exprTok{etString, str})
			i += n
		case c == '=' || c == '!' || c == '<' || c == '>':
			op, n, err := lexOp(s[i:])
			if err != nil {
				return nil, err
			}
			toks = append(toks, exprTok{etOp, op})
			i += n
		case c >= '0' && c <= '9' || (c == '-' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9'):
			num, n := lexNumber(s[i:])
			toks = append(toks, exprTok{etNumber, num})
			i += n
		case isIdentStart(rune(c)):
			id, n := lexIdent(s[i:])
			i += n
			switch id {
			case "and", "or", "not", "true", "false":
				toks = append(toks, exprTok{etKeyword, id})
			default:
				toks = append(toks, exprTok{etIdent, id})
			}
		default:
			return nil, fmt.Errorf("unexpected character %q in expression %q", string(c), s)
		}
	}
	toks = append(toks, exprTok{etEOF, ""})
	return toks, nil
}

func lexString(s string, quote byte) (string, int, error) {
	var b strings.Builder
	i := 1 // skip opening quote
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			b.WriteByte(s[i+1])
			i += 2
			continue
		}
		if s[i] == quote {
			return b.String(), i + 1, nil
		}
		b.WriteByte(s[i])
		i++
	}
	return "", 0, fmt.Errorf("unterminated string literal in expression")
}

func lexOp(s string) (string, int, error) {
	two := ""
	if len(s) >= 2 {
		two = s[:2]
	}
	switch two {
	case "==", "!=", "<=", ">=":
		return two, 2, nil
	}
	switch s[0] {
	case '<', '>':
		return string(s[0]), 1, nil
	}
	return "", 0, fmt.Errorf("invalid operator near %q", s)
}

func lexNumber(s string) (string, int) {
	i := 0
	if s[0] == '-' {
		i++
	}
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
		i++
	}
	return s[:i], i
}

func lexIdent(s string) (string, int) {
	i := 0
	for i < len(s) && isIdentPart(rune(s[i])) {
		i++
	}
	return s[:i], i
}

func isIdentStart(r rune) bool { return unicode.IsLetter(r) || r == '_' }
func isIdentPart(r rune) bool  { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

// --- expression parser (recursive descent) ---

type exprParser struct {
	toks []exprTok
	pos  int
}

// parseExpr parses a complete expression string into an Expr AST.
func parseExpr(s string) (Expr, error) {
	toks, err := lexExpr(s)
	if err != nil {
		return nil, err
	}
	p := &exprParser{toks: toks}
	e, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.cur().kind != etEOF {
		return nil, fmt.Errorf("unexpected trailing token %q in expression %q", p.cur().val, s)
	}
	return e, nil
}

func (p *exprParser) cur() exprTok  { return p.toks[p.pos] }
func (p *exprParser) advance() exprTok { t := p.toks[p.pos]; p.pos++; return t }

func (p *exprParser) parseOr() (Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.cur().kind == etKeyword && p.cur().val == "or" {
		p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &binaryExpr{op: "or", l: left, r: right}
	}
	return left, nil
}

func (p *exprParser) parseAnd() (Expr, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for p.cur().kind == etKeyword && p.cur().val == "and" {
		p.advance()
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = &binaryExpr{op: "and", l: left, r: right}
	}
	return left, nil
}

func (p *exprParser) parseNot() (Expr, error) {
	if p.cur().kind == etKeyword && p.cur().val == "not" {
		p.advance()
		x, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return &notExpr{x: x}, nil
	}
	return p.parseComparison()
}

func (p *exprParser) parseComparison() (Expr, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	if p.cur().kind == etOp {
		op := p.advance().val
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return &binaryExpr{op: op, l: left, r: right}, nil
	}
	return left, nil
}

func (p *exprParser) parsePrimary() (Expr, error) {
	t := p.cur()
	switch t.kind {
	case etLParen:
		p.advance()
		e, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.cur().kind != etRParen {
			return nil, fmt.Errorf("expected ')' in expression")
		}
		p.advance()
		return e, nil
	case etNumber:
		p.advance()
		f, err := strconv.ParseFloat(t.val, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number %q", t.val)
		}
		return &literalExpr{val: f}, nil
	case etString:
		p.advance()
		return &literalExpr{val: t.val}, nil
	case etKeyword:
		if t.val == "true" || t.val == "false" {
			p.advance()
			return &literalExpr{val: t.val == "true"}, nil
		}
		return nil, fmt.Errorf("unexpected keyword %q in expression", t.val)
	case etIdent:
		return p.parsePath()
	default:
		return nil, fmt.Errorf("unexpected token %q in expression", t.val)
	}
}

func (p *exprParser) parsePath() (Expr, error) {
	path := []string{p.advance().val}
	for p.cur().kind == etDot {
		p.advance()
		if p.cur().kind != etIdent {
			return nil, fmt.Errorf("expected identifier after '.' in path")
		}
		path = append(path, p.advance().val)
	}
	return &pathExpr{path: path}, nil
}

// --- expression AST nodes + evaluation ---

type literalExpr struct{ val any }

func (e *literalExpr) eval(map[string]any) (any, error) { return e.val, nil }

type pathExpr struct{ path []string }

func (e *pathExpr) eval(ctx map[string]any) (any, error) {
	var cur any = map[string]any(ctx)
	for _, key := range e.path {
		m, ok := cur.(map[string]any)
		if !ok {
			// Missing intermediate → nil (falsy). This makes {% if user.pro %}
			// safe even when user is absent, matching template-language norms.
			return nil, nil
		}
		cur = m[key]
	}
	return cur, nil
}

type notExpr struct{ x Expr }

func (e *notExpr) eval(ctx map[string]any) (any, error) {
	v, err := e.x.eval(ctx)
	if err != nil {
		return nil, err
	}
	return !truthy(v), nil
}

type binaryExpr struct {
	op   string
	l, r Expr
}

func (e *binaryExpr) eval(ctx map[string]any) (any, error) {
	// Short-circuit logical operators.
	if e.op == "and" || e.op == "or" {
		lv, err := e.l.eval(ctx)
		if err != nil {
			return nil, err
		}
		if e.op == "and" && !truthy(lv) {
			return false, nil
		}
		if e.op == "or" && truthy(lv) {
			return true, nil
		}
		rv, err := e.r.eval(ctx)
		if err != nil {
			return nil, err
		}
		return truthy(rv), nil
	}

	lv, err := e.l.eval(ctx)
	if err != nil {
		return nil, err
	}
	rv, err := e.r.eval(ctx)
	if err != nil {
		return nil, err
	}
	switch e.op {
	case "==":
		return valuesEqual(lv, rv), nil
	case "!=":
		return !valuesEqual(lv, rv), nil
	case "<", ">", "<=", ">=":
		return compareNumbers(e.op, lv, rv)
	}
	return nil, fmt.Errorf("unknown operator %q", e.op)
}
