import type { Node, Edge } from "@xyflow/react";
import type { WorkflowConfig, WorkflowNode, WorkflowEdge } from "@/types";
import type { NodaNodeData } from "./NodaNode";
import type { NodaEdgeData } from "./NodaEdge";

/**
 * Serialize React Flow state to Noda workflow JSON.
 */
export function serializeWorkflow(
  nodes: Node[],
  edges: Edge[]
): WorkflowConfig {
  const workflowNodes: WorkflowNode[] = nodes.map((node) => {
    const data = node.data as unknown as NodaNodeData;
    const wn: WorkflowNode = {
      id: node.id,
      type: data.nodeType,
      position: { x: Math.round(node.position.x), y: Math.round(node.position.y) },
    };
    if (data.alias) wn.as = data.alias;
    if (data.config && Object.keys(data.config).length > 0) wn.config = data.config;
    return wn;
  });

  const workflowEdges: WorkflowEdge[] = edges.map((edge) => {
    const data = edge.data as unknown as NodaEdgeData | undefined;
    const we: WorkflowEdge = {
      from: edge.source,
      output: data?.output ?? edge.sourceHandle ?? "success",
      to: edge.target,
    };
    if (data?.retry) we.retry = data.retry;
    return we;
  });

  return { nodes: workflowNodes, edges: workflowEdges };
}

/**
 * Deserialize Noda workflow JSON to React Flow state.
 */
export function deserializeWorkflow(
  workflow: WorkflowConfig,
  outputsByType: Map<string, string[]>
): { nodes: Node[]; edges: Edge[] } {
  const nodes: Node[] = workflow.nodes.map((node, index) => {
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

  const edges: Edge[] = workflow.edges.map((edge, index) => {
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
    };
  });

  return { nodes, edges };
}
