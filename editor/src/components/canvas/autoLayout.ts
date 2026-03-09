import type { WorkflowConfig, WorkflowNode } from "@/types";

const NODE_WIDTH = 180;
const NODE_HEIGHT = 100;

/**
 * Run ELKjs layout on a workflow and return updated node positions.
 * ELK is dynamically imported to avoid loading the large library upfront.
 */
export async function autoLayout(
  workflow: WorkflowConfig
): Promise<WorkflowConfig> {
  if (workflow.nodes.length === 0) return workflow;

  const ELK = (await import("elkjs/lib/elk.bundled.js")).default;
  const elk = new ELK();

  const elkGraph = {
    id: "root",
    layoutOptions: {
      "elk.algorithm": "layered",
      "elk.direction": "RIGHT",
      "elk.spacing.nodeNode": "40",
      "elk.layered.spacing.nodeNodeBetweenLayers": "60",
      "elk.layered.crossingMinimization.strategy": "LAYER_SWEEP",
    },
    children: workflow.nodes.map((node) => ({
      id: node.id,
      width: NODE_WIDTH,
      height: NODE_HEIGHT,
    })),
    edges: workflow.edges.map((edge, i) => ({
      id: `e-${i}`,
      sources: [edge.from],
      targets: [edge.to],
    })),
  };

  const layout = await elk.layout(elkGraph);

  const positionMap = new Map<string, { x: number; y: number }>();
  for (const child of layout.children ?? []) {
    positionMap.set(child.id, {
      x: Math.round(child.x ?? 0),
      y: Math.round(child.y ?? 0),
    });
  }

  const nodes: WorkflowNode[] = workflow.nodes.map((node) => ({
    ...node,
    position: positionMap.get(node.id) ?? node.position ?? { x: 0, y: 0 },
  }));

  return { ...workflow, nodes };
}
