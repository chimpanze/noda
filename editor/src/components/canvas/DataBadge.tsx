import { useState } from "react";
import type { OutputSchema } from "../../types";
import { schemaToCompactLabel, dataToCompactLabel } from "../../lib/schemaInference";
import { detectSchemaDiff, diffToLabel } from "../../lib/dataDiff";
import { useSchemaStore } from "../../stores/schema";

interface DataBadgeProps {
  data?: unknown;
  outputSchema?: OutputSchema | null;
  compact?: boolean;
  nodeId?: string;
}

export function DataBadge({ data, outputSchema, compact, nodeId }: DataBadgeProps) {
  const [expanded, setExpanded] = useState(false);

  const getPreviousSchema = useSchemaStore((s) => s.getPreviousSchema);
  const previousSchema = nodeId ? getPreviousSchema(nodeId) : null;
  const currentSchema = outputSchema?.schema;
  const diff = previousSchema && currentSchema ? detectSchemaDiff(previousSchema, currentSchema) : null;
  const diffLabel = diff ? diffToLabel(diff) : null;

  const hasData = data !== undefined;
  const label = hasData
    ? dataToCompactLabel(data)
    : outputSchema
      ? schemaToCompactLabel(outputSchema.schema)
      : null;

  if (!label) return null;

  const isStale = outputSchema?.stale ?? false;
  const isLearned = outputSchema?.source === "runtime-learned";
  const isSchemaOnly = !hasData && outputSchema;

  return (
    <div className="relative flex flex-col items-start">
      <button
        onClick={() => !compact && setExpanded(!expanded)}
        className={`
          px-1.5 py-0.5 rounded text-[10px] font-mono leading-tight max-w-[180px] truncate
          ${hasData
            ? "bg-blue-100 text-blue-800 dark:bg-blue-900/40 dark:text-blue-300"
            : isStale
              ? "bg-gray-100 text-gray-400 dark:bg-gray-800/40 dark:text-gray-500"
              : "bg-gray-100 text-gray-500 dark:bg-gray-800/40 dark:text-gray-400"
          }
          ${isLearned && !hasData ? "border border-dashed border-gray-300 dark:border-gray-600" : ""}
          ${isSchemaOnly ? "opacity-60" : ""}
          ${compact ? "text-[9px] px-1 py-0" : "cursor-pointer hover:opacity-80"}
        `}
        title={hasData ? "Click to expand data" : `Source: ${outputSchema?.source ?? "unknown"}`}
      >
        {label}
      </button>

      {diffLabel && (
        <div className="text-[9px] text-orange-500 font-mono mt-0.5 max-w-[180px] truncate" title={diffLabel}>
          {diffLabel}
        </div>
      )}

      {expanded && hasData && !compact && (
        <div
          className="absolute z-50 top-full mt-1 left-0 bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-700 rounded-md shadow-lg p-2 max-w-[400px] max-h-[300px] overflow-auto"
          onClick={(e) => e.stopPropagation()}
        >
          <pre className="text-[11px] font-mono text-gray-700 dark:text-gray-300 whitespace-pre-wrap">
            {JSON.stringify(data, null, 2)}
          </pre>
        </div>
      )}
    </div>
  );
}
