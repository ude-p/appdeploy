package render

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
)

func referenceName(expression hcl.Expression) (string, error) {
	traversal, diagnostics := hcl.AbsTraversalForExpr(expression)
	if diagnostics.HasErrors() || len(traversal) != 2 {
		return "", fmt.Errorf("must be a resource reference")
	}
	name, ok := traversal[1].(hcl.TraverseAttr)
	if !ok {
		return "", fmt.Errorf("must be a resource reference")
	}
	return name.Name, nil
}
