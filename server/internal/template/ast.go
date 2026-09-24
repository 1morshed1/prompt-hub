package template

// Node is one element of the parsed template tree. The evaluator (eval.go) walks
// a []Node and appends to an output buffer. There are five concrete node types,
// mirroring the five things the language can express.
type Node interface{ node() }

// TextNode is literal text emitted verbatim.
type TextNode struct{ Text string }

// OutputNode is a {{ expr }} substitution. Expr is a full expression (so dotted
// paths like user.name and even comparisons work), evaluated then stringified.
type OutputNode struct{ Expr Expr }

// IfNode is an if / elif* / else? / endif block. Branches are tried in order; the
// first whose Cond is truthy renders. Else renders if no branch matches.
type IfNode struct {
	Branches []Branch
	Else     []Node
}

// Branch is one (condition, body) pair inside an IfNode.
type Branch struct {
	Cond Expr
	Body []Node
}

// ForNode is a {% for Var in Iter %}...{% endfor %} loop. Body is rendered once
// per element of Iter, with Var bound to the element in a child scope.
type ForNode struct {
	Var  string
	Iter Expr
	Body []Node
}

// IncludeNode is {% include "name" %}. The name is resolved to source at eval time
// via the engine's IncludeResolver, then parsed and rendered in the same context.
type IncludeNode struct{ Name string }

func (TextNode) node()    {}
func (OutputNode) node()  {}
func (IfNode) node()      {}
func (ForNode) node()     {}
func (IncludeNode) node() {}
