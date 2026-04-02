import {
  BaseEdge,
  getBezierPath,
  type EdgeProps,
  EdgeLabelRenderer,
} from "@xyflow/react";
import { useTraceStore } from "@/stores/trace";
import { DataBadge } from "./DataBadge";

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
  source,
  data,
  selected,
}: EdgeProps) {
  const edgeData = data as unknown as NodaEdgeData | undefined;
  const isError = edgeData?.output === "error";
  const hasRetry = edgeData?.retry != null;
  const output = edgeData?.output ?? "success";

  // Get live data from the source node for this edge
  const activeExecution = useTraceStore((s) => s.getActiveExecution());
  const sourceNodeData = activeExecution?.nodeData.get(source);
  const liveData = sourceNodeData?.data;

  // Check if this edge is active during trace
  const activeEdgeKeys = useTraceStore((s) => s.activeEdgeKeys);
  const edgeKey = `${source}:${output}`;
  const isActive = activeEdgeKeys.has(edgeKey);
  const isErrorActive = isError && isActive;
  const isSuccessActive = !isError && isActive;

  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    targetX,
    targetY,
    sourcePosition,
    targetPosition,
  });

  // Determine stroke color
  let strokeColor = isError ? "#ef4444" : "#64748b";
  if (isSuccessActive) strokeColor = "#22c55e";
  if (isErrorActive) strokeColor = "#ef4444";

  return (
    <>
      {/* Glow effect for active edges */}
      {isActive && (
        <BaseEdge
          path={edgePath}
          style={{
            stroke: isErrorActive ? "#ef4444" : "#22c55e",
            strokeWidth: 6,
            strokeOpacity: 0.3,
            filter: "blur(3px)",
          }}
        />
      )}
      <BaseEdge
        path={edgePath}
        style={{
          stroke: strokeColor,
          strokeWidth: selected ? 3 : isActive ? 2.5 : 2,
          strokeDasharray: isError ? "6 3" : undefined,
        }}
      />
      {/* Animated flow indicator */}
      {isActive && (
        <circle r="3" fill={isErrorActive ? "#ef4444" : "#22c55e"}>
          <animateMotion dur="1s" repeatCount="indefinite" path={edgePath} />
        </circle>
      )}
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
      {liveData !== undefined && (
        <EdgeLabelRenderer>
          <div
            style={{
              position: "absolute",
              transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
              pointerEvents: "all",
            }}
            className="nodrag nopan"
          >
            <DataBadge data={liveData} compact />
          </div>
        </EdgeLabelRenderer>
      )}
    </>
  );
}
