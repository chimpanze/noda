import { useCallback, useMemo, useRef } from "react";
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  useReactFlow,
  type Node,
  type Edge,
  type Connection,
} from "@xyflow/react";
import { useEditorStore } from "@/stores/editor";
import { NodaNode } from "./NodaNode";
import { NodaEdge } from "./NodaEdge";
import type { NodaNodeData } from "./NodaNode";
import type { NodaEdgeData } from "./NodaEdge";
import { getOutputColor } from "./nodeStyles";

const nodeTypes = { noda: NodaNode };
const edgeTypes = { noda: NodaEdge };

function generateNodeId(nodeType: string, existingIds: string[]): string {
  const prefix = nodeType.replace(/\./g, "-");
  // Find highest numeric suffix for this prefix among existing nodes
  let max = 0;
  for (const id of existingIds) {
    if (id.startsWith(prefix + "-")) {
      const num = parseInt(id.slice(prefix.length + 1), 10);
      if (!isNaN(num) && num > max) max = num;
    }
  }
  return `${prefix}-${max + 1}`;
}

export function WorkflowCanvas() {
  const activeWorkflow = useEditorStore((s) => s.activeWorkflow);
  const selectNode = useEditorStore((s) => s.selectNode);
  const selectEdge = useEditorStore((s) => s.selectEdge);
  const deselectAll = useEditorStore((s) => s.deselectAll);
  const nodeTypeRegistry = useEditorStore((s) => s.nodeTypes);
  const addNode = useEditorStore((s) => s.addNode);
  const addEdge = useEditorStore((s) => s.addEdge);
  const updateNodePosition = useEditorStore((s) => s.updateNodePosition);

  const reactFlowWrapper = useRef<HTMLDivElement>(null);
  const { screenToFlowPosition } = useReactFlow();

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
          stroke:
            edge.output === "error"
              ? "#ef4444"
              : getOutputColor(edge.output),
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

  const onEdgeClick = useCallback(
    (_: React.MouseEvent, edge: Edge) => {
      // Extract index from edge id format "e-{index}"
      const index = parseInt(edge.id.replace("e-", ""), 10);
      if (!isNaN(index)) selectEdge(index);
    },
    [selectEdge]
  );

  const onPaneClick = useCallback(() => {
    deselectAll();
  }, [deselectAll]);

  // Edge connection handler
  const onConnect = useCallback(
    (connection: Connection) => {
      if (!connection.source || !connection.target) return;
      // Prevent self-connection
      if (connection.source === connection.target) return;
      const output = connection.sourceHandle ?? "success";
      addEdge({
        from: connection.source,
        output,
        to: connection.target,
      });
    },
    [addEdge]
  );

  // Node position update on drag end
  const onNodeDragStop = useCallback(
    (_: React.MouseEvent, node: Node) => {
      updateNodePosition(node.id, node.position);
    },
    [updateNodePosition]
  );

  // Drag-and-drop from palette
  const onDragOver = useCallback((event: React.DragEvent) => {
    event.preventDefault();
    event.dataTransfer.dropEffect = "move";
  }, []);

  const onDrop = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault();
      const nodeType = event.dataTransfer.getData("application/noda-node-type");
      if (!nodeType) return;

      const position = screenToFlowPosition({
        x: event.clientX,
        y: event.clientY,
      });

      const existingIds = activeWorkflow?.nodes.map((n) => n.id) ?? [];
      const newNode = {
        id: generateNodeId(nodeType, existingIds),
        type: nodeType,
        position,
        config: {},
      };

      addNode(newNode);
      selectNode(newNode.id);
    },
    [screenToFlowPosition, addNode, selectNode, activeWorkflow?.nodes]
  );

  if (!activeWorkflow) {
    return (
      <div className="flex-1 flex items-center justify-center text-gray-400 text-sm">
        Select a workflow to view it on the canvas.
      </div>
    );
  }

  return (
    <div className="flex-1" ref={reactFlowWrapper}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        onNodeClick={onNodeClick}
        onEdgeClick={onEdgeClick}
        onPaneClick={onPaneClick}
        onConnect={onConnect}
        onNodeDragStop={onNodeDragStop}
        onDragOver={onDragOver}
        onDrop={onDrop}
        fitView
        minZoom={0.1}
        maxZoom={2}
        deleteKeyCode="Delete"
      >
        <Background gap={16} size={1} />
        <Controls />
        <MiniMap nodeStrokeWidth={3} pannable zoomable />
      </ReactFlow>
    </div>
  );
}
