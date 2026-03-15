package engine

import (
	"fmt"
	"strings"
)

// NodeExecutionError wraps a node execution failure with context about which
// node failed and what data was available at the time.
type NodeExecutionError struct {
	NodeID         string
	NodeType       string
	Err            error
	AvailableNodes []string
}

func (e *NodeExecutionError) Error() string {
	return fmt.Sprintf("node %q (%s): %s [available nodes: %s]",
		e.NodeID, e.NodeType, e.Err.Error(), strings.Join(e.AvailableNodes, ", "))
}

func (e *NodeExecutionError) Unwrap() error {
	return e.Err
}
