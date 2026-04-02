import { Handle, Position, type NodeProps } from "@xyflow/react";
import { AlertCircle, Loader2 } from "lucide-react";
import { getCategoryStyle, getOutputColor } from "./nodeStyles";
import { DataBadge } from "./DataBadge";
import { useEditorStore } from "@/stores/editor";
import { useSchemaStore } from "@/stores/schema";
import { useTraceStore } from "@/stores/trace";
import type { NodeExecState } from "@/types";

export interface NodaNodeData {
  nodeType: string;
  label: string;
  alias?: string;
  outputs: string[];
  config?: Record<string, unknown>;
  [key: string]: unknown;
}

const execStateStyles: Record<NodeExecState, string> = {
  idle: "",
  running: "ring-2 ring-blue-400 ring-offset-1 animate-pulse",
  completed: "ring-2 ring-green-400 ring-offset-1",
  failed: "ring-2 ring-red-400 ring-offset-1",
};

export function NodaNode({ data, selected, id }: NodeProps) {
  const nodeData = data as unknown as NodaNodeData;
  const style = getCategoryStyle(nodeData.nodeType);
  const outputs = nodeData.outputs ?? ["success", "error"];
  const validationErrors = useEditorStore((s) => s.validationErrors);
  const activeTraceId = useTraceStore((s) => s.activeTraceId);
  const getNodeState = useTraceStore((s) => s.getNodeState);
  const getActiveExecution = useTraceStore((s) => s.getActiveExecution);
  const getNodeOutputSchema = useSchemaStore((s) => s.getNodeOutputSchema);
  const activeWorkflow = useEditorStore((s) => s.activeWorkflow);

  // Validation errors
  const nodeErrors = validationErrors.filter(
    (e) => e.path?.includes(id) || e.message?.includes(id),
  );
  const hasValidationError = nodeErrors.length > 0;

  // Execution state
  const execState: NodeExecState = activeTraceId
    ? getNodeState(activeTraceId, id)
    : "idle";

  const execRing = execStateStyles[execState];

  // Live data from active trace execution
  const activeExecution = getActiveExecution();
  const nodeTraceData = activeExecution?.nodeData.get(id);
  const liveData = nodeTraceData?.data;

  // Schema for this node
  const nodeConfig = activeWorkflow?.nodes.find((n) => n.id === id)?.config as Record<string, unknown> | undefined;
  const outputSchema = getNodeOutputSchema(id, nodeData.nodeType, nodeConfig);

  return (
    <div
      className={`rounded-lg border-2 shadow-sm min-w-[160px] ${style.bg} ${
        hasValidationError ? "border-red-400" : style.border
      } ${selected && execState === "idle" ? "ring-2 ring-blue-400 ring-offset-1" : ""} ${execRing}`}
    >
      {/* Input handle */}
      <Handle
        type="target"
        position={Position.Left}
        className="!w-3 !h-3 !bg-gray-400 !border-2 !border-white"
      />

      {/* Header */}
      <div className="px-3 py-2 border-b border-inherit">
        <div className="flex items-center justify-between">
          <div className={`text-xs font-mono ${style.iconColor}`}>
            {nodeData.nodeType}
          </div>
          <div className="flex items-center gap-1">
            {execState === "running" && (
              <Loader2 size={14} className="text-blue-500 animate-spin" />
            )}
            {execState === "failed" && (
              <AlertCircle size={14} className="text-red-500" />
            )}
            {hasValidationError && execState !== "failed" && (
              <span title={nodeErrors.map((e) => e.message).join("\n")}>
                <AlertCircle size={14} className="text-red-500" />
              </span>
            )}
          </div>
        </div>
        <div className={`text-sm font-medium ${style.text}`}>
          {nodeData.alias ?? nodeData.label}
        </div>
      </div>

      {/* Config preview */}
      {nodeData.config && Object.keys(nodeData.config).length > 0 && (
        <div className="px-3 py-1.5 text-xs text-gray-500 truncate max-w-[200px]">
          {summarizeConfig(nodeData.config)}
        </div>
      )}

      {/* Output ports */}
      <div className="px-3 py-1.5 space-y-1">
        {outputs.map((output, index) => (
          <div
            key={output}
            className="flex items-center justify-end gap-1.5 relative"
          >
            <span className="text-xs text-gray-500">{output}</span>
            <Handle
              type="source"
              position={Position.Right}
              id={output}
              className="!w-3 !h-3 !border-2 !border-white"
              style={{
                backgroundColor: getOutputColor(output),
                top: "auto",
                position: "relative",
                transform: "none",
                right: "-12px",
              }}
              isConnectable
            />
            <Handle
              type="source"
              position={Position.Right}
              id={`${output}-abs`}
              style={{
                opacity: 0,
                top: `${((index + 0.5) / outputs.length) * 100}%`,
                pointerEvents: "none",
              }}
              isConnectable={false}
            />
          </div>
        ))}
      </div>

      {/* Data badge */}
      <div className="px-3 pb-1.5">
        <DataBadge data={liveData} outputSchema={outputSchema} />
      </div>
    </div>
  );
}

function summarizeConfig(config: Record<string, unknown>): string {
  for (const key of [
    "query",
    "sql",
    "condition",
    "expression",
    "url",
    "template",
    "key",
    "channel",
    "topic",
  ]) {
    if (key in config && typeof config[key] === "string") {
      const val = config[key] as string;
      return val.length > 40 ? val.slice(0, 37) + "..." : val;
    }
  }
  const keys = Object.keys(config);
  return keys.length <= 3 ? keys.join(", ") : `${keys.length} fields`;
}
