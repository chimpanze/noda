import { useCallback, useMemo } from "react";
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  type Node,
  type Edge,
} from "@xyflow/react";
import { useEditorStore } from "@/stores/editor";
import { NodaNode } from "./NodaNode";
import { NodaEdge } from "./NodaEdge";
import type { NodaNodeData } from "./NodaNode";
import type { NodaEdgeData } from "./NodaEdge";
import { getOutputColor } from "./nodeStyles";

const nodeTypes = { noda: NodaNode };
const edgeTypes = { noda: NodaEdge };

export function WorkflowCanvas() {
  const activeWorkflow = useEditorStore((s) => s.activeWorkflow);
  const selectNode = useEditorStore((s) => s.selectNode);
  const deselectAll = useEditorStore((s) => s.deselectAll);
  const nodeTypeRegistry = useEditorStore((s) => s.nodeTypes);

  // Build a lookup: nodeType → outputs
  const outputsByType = useMemo(() => {
    const map = new Map<string, string[]>();
    for (const nt of nodeTypeRegistry) {
      map.set(nt.type, nt.outputs);
    }
    return map;
  }, [nodeTypeRegistry]);

  // Convert Noda workflow nodes to React Flow nodes
  const nodes: Node[] = useMemo(() => {
    if (!activeWorkflow?.nodes) return [];

    return activeWorkflow.nodes.map((node, index) => {
      const outputs = outputsByType.get(node.type) ?? ["success", "error"];
      const data: NodaNodeData = {
        nodeType: node.type,
        label: node.id,
        alias: node.as,
        outputs,
        config: node.config,
      };

      return {
        id: node.id,
        type: "noda",
        position: node.position ?? { x: 50, y: index * 120 },
        data,
      };
    });
  }, [activeWorkflow?.nodes, outputsByType]);

  // Convert Noda workflow edges to React Flow edges
  const edges: Edge[] = useMemo(() => {
    if (!activeWorkflow?.edges) return [];

    return activeWorkflow.edges.map((edge, index) => {
      const data: NodaEdgeData = {
        output: edge.output,
        retry: edge.retry,
      };

      return {
        id: `e-${index}`,
        source: edge.from,
        target: edge.to,
        sourceHandle: edge.output,
        type: "noda",
        data,
        style: {
          stroke: edge.output === "error" ? "#ef4444" : getOutputColor(edge.output),
        },
      };
    });
  }, [activeWorkflow?.edges]);

  const onNodeClick = useCallback(
    (_: React.MouseEvent, node: Node) => {
      selectNode(node.id);
    },
    [selectNode]
  );

  const onPaneClick = useCallback(() => {
    deselectAll();
  }, [deselectAll]);

  if (!activeWorkflow) {
    return (
      <div className="flex-1 flex items-center justify-center text-gray-400 text-sm">
        Select a workflow to view it on the canvas.
      </div>
    );
  }

  return (
    <div className="flex-1">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        onNodeClick={onNodeClick}
        onPaneClick={onPaneClick}
        fitView
        minZoom={0.1}
        maxZoom={2}
      >
        <Background gap={16} size={1} />
        <Controls />
        <MiniMap
          nodeStrokeWidth={3}
          pannable
          zoomable
        />
      </ReactFlow>
    </div>
  );
}
