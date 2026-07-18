package expr

import (
	"strings"

	"github.com/expr-lang/expr/ast"
)

// headerKeyPatcher lowercases constant string keys used to index a `headers`
// map, e.g. headers['X-GitHub-Event'] → headers['x-github-event']. Inbound
// trigger headers and http.* response headers store lowercase keys; this keeps
// any-case config lookups working. Dynamic keys are left alone and must be
// lowercase.
type headerKeyPatcher struct{}

func (headerKeyPatcher) Visit(node *ast.Node) {
	member, ok := (*node).(*ast.MemberNode)
	if !ok {
		return
	}
	str, ok := member.Property.(*ast.StringNode)
	if !ok || !isHeadersBase(member.Node) {
		return
	}
	if lower := strings.ToLower(str.Value); lower != str.Value {
		patched := &ast.StringNode{Value: lower}
		patched.SetLocation(str.Location())
		member.Property = patched
	}
}

// isHeadersBase reports whether the indexed expression is itself a reference
// named "headers" — bare (`headers[...]`) or via member access
// (`request.headers[...]`, `input.headers[...]`, `nodes.fetch.headers[...]`).
func isHeadersBase(n ast.Node) bool {
	switch b := n.(type) {
	case *ast.IdentifierNode:
		return b.Value == "headers"
	case *ast.MemberNode:
		prop, ok := b.Property.(*ast.StringNode)
		return ok && prop.Value == "headers"
	}
	return false
}
