import { memo } from "react";
import { Handle, Position, type NodeProps } from "@xyflow/react";
import { Key, Link } from "lucide-react";
import type { ColumnDef } from "@/types";

interface TableNodeData {
  label: string;
  columns: Record<string, ColumnDef>;
  relations?: Record<string, { foreign_key: string }>;
  [key: string]: unknown;
}

function TableNodeComponent({ data }: NodeProps) {
  const d = data as unknown as TableNodeData;
  const columns = Object.entries(d.columns ?? {});

  // Collect FK column names
  const fkCols = new Set<string>();
  if (d.relations) {
    for (const rel of Object.values(d.relations)) {
      if (rel.foreign_key) fkCols.add(rel.foreign_key);
    }
  }

  return (
    <div className="bg-white border-2 border-gray-300 rounded-lg shadow-sm min-w-[180px] overflow-hidden">
      <Handle type="target" position={Position.Left} className="!bg-blue-400 !w-2 !h-2" />
      <div className="bg-blue-500 text-white px-3 py-1.5 text-sm font-semibold">
        {d.label}
      </div>
      <div className="divide-y divide-gray-100">
        {columns.map(([name, col]) => (
          <div key={name} className="flex items-center gap-1.5 px-3 py-1 text-xs">
            {col.primary_key ? (
              <Key size={10} className="text-yellow-500 shrink-0" />
            ) : fkCols.has(name) ? (
              <Link size={10} className="text-blue-400 shrink-0" />
            ) : (
              <span className="w-2.5 shrink-0" />
            )}
            <span className="font-mono text-gray-800">{name}</span>
            <span className="text-gray-400 ml-auto">{col.type}</span>
          </div>
        ))}
      </div>
      <Handle type="source" position={Position.Right} className="!bg-blue-400 !w-2 !h-2" />
    </div>
  );
}

export const TableNode = memo(TableNodeComponent);
