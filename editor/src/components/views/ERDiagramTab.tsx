import { useMemo, useCallback } from "react";
import {
  ReactFlow,
  Background,
  Controls,
  type Node,
  type Edge,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { TableNode } from "./TableNode";
import type { ModelDefinition } from "@/types";

const nodeTypes = { tableNode: TableNode };

interface Props {
  models: { path: string; model: ModelDefinition }[];
  onSelectModel: (path: string) => void;
}

export function ERDiagramTab({ models, onSelectModel }: Props) {
  const { nodes, edges } = useMemo(() => {
    const ns: Node[] = [];
    const es: Edge[] = [];
    const COLS_PER_ROW = 3;
    const NODE_W = 220;
    const NODE_H_BASE = 60;
    const ROW_H = 18;
    const GAP_X = 80;
    const GAP_Y = 60;

    models.forEach((m, i) => {
      const colCount = Object.keys(m.model.columns).length;
      const x = (i % COLS_PER_ROW) * (NODE_W + GAP_X);
      const y = Math.floor(i / COLS_PER_ROW) * (NODE_H_BASE + colCount * ROW_H + GAP_Y);

      ns.push({
        id: m.model.table,
        type: "tableNode",
        position: { x, y },
        data: {
          label: m.model.table,
          columns: m.model.columns,
          relations: m.model.relations,
        },
      });

      // Build edges from relations
      if (m.model.relations) {
        for (const [name, rel] of Object.entries(m.model.relations)) {
          if (rel.type === "belongsTo") {
            es.push({
              id: `${m.model.table}-${name}`,
              source: m.model.table,
              target: rel.table,
              label: "N:1",
              type: "smoothstep",
              animated: false,
              style: { stroke: "#94a3b8" },
            });
          } else if (rel.type === "hasMany") {
            es.push({
              id: `${m.model.table}-${name}`,
              source: rel.table,
              target: m.model.table,
              label: "1:N",
              type: "smoothstep",
              animated: false,
              style: { stroke: "#94a3b8", strokeDasharray: "5,5" },
            });
          } else if (rel.type === "manyToMany") {
            es.push({
              id: `${m.model.table}-${name}`,
              source: m.model.table,
              target: rel.table,
              label: "M:N",
              type: "smoothstep",
              animated: true,
              style: { stroke: "#818cf8" },
            });
          }
        }
      }
    });

    return { nodes: ns, edges: es };
  }, [models]);

  const onNodeClick = useCallback(
    (_: React.MouseEvent, node: Node) => {
      const m = models.find((m) => m.model.table === node.id);
      if (m) onSelectModel(m.path);
    },
    [models, onSelectModel]
  );

  if (models.length === 0) {
    return (
      <div className="flex-1 flex items-center justify-center text-sm text-gray-400">
        No models to display. Create a model first.
      </div>
    );
  }

  return (
    <div className="flex-1" style={{ height: "100%" }}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        onNodeClick={onNodeClick}
        fitView
        proOptions={{ hideAttribution: true }}
      >
        <Background />
        <Controls />
      </ReactFlow>
    </div>
  );
}
