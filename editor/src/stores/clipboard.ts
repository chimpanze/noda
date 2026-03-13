import type { WorkflowNode, WorkflowEdge } from "@/types";

interface ClipboardContent {
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
}

let clipboard: ClipboardContent | null = null;
let pasteCounter = 0;

export function copyNodes(
  allNodes: WorkflowNode[],
  allEdges: WorkflowEdge[],
  selectedIds: Set<string>,
) {
  if (selectedIds.size === 0) return;

  const nodes = allNodes.filter((n) => selectedIds.has(n.id));
  // Only include edges where both endpoints are in the selection
  const edges = allEdges.filter(
    (e) => selectedIds.has(e.from) && selectedIds.has(e.to),
  );

  clipboard = { nodes: structuredClone(nodes), edges: structuredClone(edges) };
  pasteCounter = 0;
}

export function pasteNodes(): {
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
} | null {
  if (!clipboard || clipboard.nodes.length === 0) return null;

  pasteCounter++;
  const offset = pasteCounter * 20;

  // Build ID mapping: old → new
  const idMap = new Map<string, string>();
  for (const node of clipboard.nodes) {
    idMap.set(node.id, `${node.id}-copy-${pasteCounter}`);
  }

  const nodes: WorkflowNode[] = clipboard.nodes.map((n) => ({
    ...structuredClone(n),
    id: idMap.get(n.id)!,
    position: n.position
      ? { x: n.position.x + offset, y: n.position.y + offset }
      : undefined,
  }));

  const edges: WorkflowEdge[] = clipboard.edges.map((e) => ({
    ...structuredClone(e),
    from: idMap.get(e.from) ?? e.from,
    to: idMap.get(e.to) ?? e.to,
  }));

  return { nodes, edges };
}

export function hasClipboard(): boolean {
  return clipboard !== null && clipboard.nodes.length > 0;
}
