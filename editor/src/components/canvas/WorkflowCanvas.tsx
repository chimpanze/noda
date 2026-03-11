import { useCallback, useMemo, useRef, useState } from "react";
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  useReactFlow,
  type Node,
  type Edge,
  type Connection,
  type OnSelectionChangeParams,
} from "@xyflow/react";
import { useEditorStore } from "@/stores/editor";
import { NodaNode } from "./NodaNode";
import { NodaEdge } from "./NodaEdge";
import type { NodaNodeData } from "./NodaNode";
import type { NodaEdgeData } from "./NodaEdge";
import { getOutputColor } from "./nodeStyles";
import { CanvasContextMenu, type ContextMenuState } from "./CanvasContextMenu";
import { copyNodes, pasteNodes, hasClipboard } from "@/stores/clipboard";
import { autoLayout } from "./autoLayout";
import { QuickAddDialog } from "./QuickAddDialog";

const rfNodeTypes = { noda: NodaNode };
const rfEdgeTypes = { noda: NodaEdge };

/**
 * Compute outputs for a node, taking config into account for dynamic types.
 * Falls back to static registry outputs.
 */
function computeNodeOutputs(
  nodeType: string,
  config: Record<string, unknown> | undefined,
  staticOutputs: Map<string, string[]>
): string[] {
  // control.switch: outputs are derived from cases config
  if (nodeType === "control.switch" && config?.cases) {
    const cases = config.cases;
    if (Array.isArray(cases)) {
      const outputs = cases.filter((c): c is string => typeof c === "string");
      return [...outputs, "default", "error"];
    }
  }

  return staticOutputs.get(nodeType) ?? ["success", "error"];
}

function generateNodeId(nodeType: string, existingIds: string[]): string {
  const prefix = nodeType.replace(/\./g, "-");
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
  const selectedNodeIds = useEditorStore((s) => s.selectedNodeIds);
  const setSelectedNodeIds = useEditorStore((s) => s.setSelectedNodeIds);
  const removeNode = useEditorStore((s) => s.removeNode);
  const removeSelectedNodes = useEditorStore((s) => s.removeSelectedNodes);
  const removeEdge = useEditorStore((s) => s.removeEdge);
  const updateNodePosition = useEditorStore((s) => s.updateNodePosition);
  const updateEdgeRetry = useEditorStore((s) => s.updateEdgeRetry);
  const setWorkflow = useEditorStore((s) => s.setWorkflow);

  const reactFlowWrapper = useRef<HTMLDivElement>(null);
  const { screenToFlowPosition } = useReactFlow();

  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null);
  const [quickAdd, setQuickAdd] = useState<{ x: number; y: number } | null>(null);

  // Build a lookup: nodeType → static outputs (fallback)
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
      const outputs = computeNodeOutputs(node.type, node.config, outputsByType);
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
      const index = parseInt(edge.id.replace("e-", ""), 10);
      if (!isNaN(index)) selectEdge(index);
    },
    [selectEdge]
  );

  const onPaneClick = useCallback(() => {
    deselectAll();
    setContextMenu(null);
  }, [deselectAll]);

  const onSelectionChange = useCallback(
    ({ nodes: selectedNodes }: OnSelectionChangeParams) => {
      if (selectedNodes.length > 0) {
        setSelectedNodeIds(new Set(selectedNodes.map((n) => n.id)));
      }
    },
    [setSelectedNodeIds]
  );

  // Context menu handlers
  const onNodeContextMenu = useCallback(
    (event: React.MouseEvent, node: Node) => {
      event.preventDefault();
      selectNode(node.id);
      setContextMenu({ type: "node", x: event.clientX, y: event.clientY, targetId: node.id });
    },
    [selectNode]
  );

  const onEdgeContextMenu = useCallback(
    (event: React.MouseEvent, edge: Edge) => {
      event.preventDefault();
      const index = parseInt(edge.id.replace("e-", ""), 10);
      if (!isNaN(index)) selectEdge(index);
      setContextMenu({ type: "edge", x: event.clientX, y: event.clientY, targetId: edge.id });
    },
    [selectEdge]
  );

  const onPaneContextMenu = useCallback(
    (event: MouseEvent | React.MouseEvent) => {
      event.preventDefault();
      setContextMenu({ type: "pane", x: event.clientX, y: event.clientY });
    },
    []
  );

  // Context menu action handlers — use multi-select if available, else target node
  const handleCopyNode = useCallback(() => {
    if (!activeWorkflow) return;
    const ids = selectedNodeIds.size > 1 ? selectedNodeIds : contextMenu?.targetId ? new Set([contextMenu.targetId]) : new Set<string>();
    if (ids.size === 0) return;
    copyNodes(activeWorkflow.nodes, activeWorkflow.edges, ids);
  }, [activeWorkflow, contextMenu, selectedNodeIds]);

  const handleDuplicateNode = useCallback(() => {
    if (!activeWorkflow) return;
    const ids = selectedNodeIds.size > 1 ? selectedNodeIds : contextMenu?.targetId ? new Set([contextMenu.targetId]) : new Set<string>();
    if (ids.size === 0) return;
    copyNodes(activeWorkflow.nodes, activeWorkflow.edges, ids);
    const result = pasteNodes();
    if (result) {
      for (const n of result.nodes) addNode(n);
      for (const e of result.edges) addEdge(e);
      if (result.nodes.length > 0) selectNode(result.nodes[0].id);
    }
  }, [activeWorkflow, contextMenu, selectedNodeIds, addNode, addEdge, selectNode]);

  const handleDeleteNode = useCallback(() => {
    if (selectedNodeIds.size > 1) {
      removeSelectedNodes();
    } else if (contextMenu?.targetId) {
      removeNode(contextMenu.targetId);
    }
  }, [contextMenu, selectedNodeIds, removeNode, removeSelectedNodes]);

  const handleToggleRetry = useCallback(() => {
    if (!contextMenu?.targetId || !activeWorkflow) return;
    const edgeIndex = parseInt(contextMenu.targetId.replace("e-", ""), 10);
    if (isNaN(edgeIndex)) return;
    const edge = activeWorkflow.edges[edgeIndex];
    if (!edge) return;
    if (edge.retry) {
      updateEdgeRetry(edgeIndex, undefined);
    } else {
      updateEdgeRetry(edgeIndex, { attempts: 3, delay: "1s" });
    }
  }, [contextMenu, activeWorkflow, updateEdgeRetry]);

  const handleDeleteEdge = useCallback(() => {
    if (!contextMenu?.targetId || !activeWorkflow) return;
    const edgeIndex = parseInt(contextMenu.targetId.replace("e-", ""), 10);
    if (isNaN(edgeIndex)) return;
    const edge = activeWorkflow.edges[edgeIndex];
    if (edge) removeEdge(edge.from, edge.output, edge.to);
  }, [contextMenu, activeWorkflow, removeEdge]);

  const handlePaste = useCallback(() => {
    const result = pasteNodes();
    if (result) {
      for (const n of result.nodes) addNode(n);
      for (const e of result.edges) addEdge(e);
      if (result.nodes.length > 0) selectNode(result.nodes[0].id);
    }
  }, [addNode, addEdge, selectNode]);

  const handleQuickAdd = useCallback(
    (nodeType: string) => {
      if (!quickAdd) return;
      const position = screenToFlowPosition({ x: quickAdd.x, y: quickAdd.y });
      const existingIds = activeWorkflow?.nodes.map((n) => n.id) ?? [];
      const newNode = {
        id: generateNodeId(nodeType, existingIds),
        type: nodeType,
        position,
        config: {},
      };
      addNode(newNode);
      selectNode(newNode.id);
      setQuickAdd(null);
    },
    [quickAdd, screenToFlowPosition, activeWorkflow?.nodes, addNode, selectNode]
  );

  const handleAutoLayout = useCallback(async () => {
    if (!activeWorkflow) return;
    const layouted = await autoLayout(activeWorkflow);
    setWorkflow(layouted);
  }, [activeWorkflow, setWorkflow]);

  const getEdgeRetryState = useCallback(() => {
    if (!contextMenu?.targetId || !activeWorkflow) return false;
    const edgeIndex = parseInt(contextMenu.targetId.replace("e-", ""), 10);
    if (isNaN(edgeIndex)) return false;
    return !!activeWorkflow.edges[edgeIndex]?.retry;
  }, [contextMenu, activeWorkflow]);

  // Edge connection handler
  const onConnect = useCallback(
    (connection: Connection) => {
      if (!connection.source || !connection.target) return;
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

  // Node position update on drag end (handles single + multi-select drag)
  const onNodeDragStop = useCallback(
    (_: React.MouseEvent, _node: Node, nodes: Node[]) => {
      for (const n of nodes) {
        updateNodePosition(n.id, n.position);
      }
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
        nodeTypes={rfNodeTypes}
        edgeTypes={rfEdgeTypes}
        onNodeClick={onNodeClick}
        onEdgeClick={onEdgeClick}
        onPaneClick={onPaneClick}
        onNodeContextMenu={onNodeContextMenu}
        onEdgeContextMenu={onEdgeContextMenu}
        onPaneContextMenu={onPaneContextMenu}
        onConnect={onConnect}
        onNodeDragStop={onNodeDragStop}
        onDragOver={onDragOver}
        onDrop={onDrop}
        onSelectionChange={onSelectionChange}
        selectionOnDrag
        fitView
        minZoom={0.1}
        maxZoom={2}
        deleteKeyCode={null}
      >
        <Background gap={16} size={1} />
        <Controls />
        <MiniMap nodeStrokeWidth={3} pannable zoomable />
      </ReactFlow>

      {quickAdd && (
        <QuickAddDialog
          x={quickAdd.x}
          y={quickAdd.y}
          onAdd={handleQuickAdd}
          onClose={() => setQuickAdd(null)}
        />
      )}

      {contextMenu && (
        <CanvasContextMenu
          menu={contextMenu}
          onClose={() => setContextMenu(null)}
          onCopyNode={handleCopyNode}
          onDuplicateNode={handleDuplicateNode}
          onDeleteNode={handleDeleteNode}
          onToggleRetry={handleToggleRetry}
          onDeleteEdge={handleDeleteEdge}
          hasRetry={getEdgeRetryState()}
          onAddNode={() => {
            if (contextMenu) setQuickAdd({ x: contextMenu.x, y: contextMenu.y });
          }}
          onPaste={handlePaste}
          onAutoLayout={handleAutoLayout}
          canPaste={hasClipboard()}
        />
      )}
    </div>
  );
}
