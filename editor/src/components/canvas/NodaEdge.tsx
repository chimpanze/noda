import {
  BaseEdge,
  getBezierPath,
  type EdgeProps,
  EdgeLabelRenderer,
} from "@xyflow/react";

export interface NodaEdgeData {
  output: string;
  retry?: { attempts: number; delay: string };
  [key: string]: unknown;
}

export function NodaEdge({
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  data,
  selected,
}: EdgeProps) {
  const edgeData = data as unknown as NodaEdgeData | undefined;
  const isError = edgeData?.output === "error";
  const hasRetry = edgeData?.retry != null;

  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    targetX,
    targetY,
    sourcePosition,
    targetPosition,
  });

  return (
    <>
      <BaseEdge
        path={edgePath}
        style={{
          stroke: isError ? "#ef4444" : "#64748b",
          strokeWidth: selected ? 3 : 2,
          strokeDasharray: isError ? "6 3" : undefined,
        }}
      />
      {hasRetry && (
        <EdgeLabelRenderer>
          <div
            className="absolute bg-amber-100 text-amber-800 text-xs px-1.5 py-0.5 rounded-full border border-amber-300 pointer-events-none"
            style={{
              transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
            }}
          >
            retry ×{edgeData!.retry!.attempts}
          </div>
        </EdgeLabelRenderer>
      )}
    </>
  );
}
