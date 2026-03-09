import { Handle, Position, type NodeProps } from "@xyflow/react";
import { AlertCircle } from "lucide-react";
import { getCategoryStyle, getOutputColor } from "./nodeStyles";
import { useEditorStore } from "@/stores/editor";

export interface NodaNodeData {
  nodeType: string;
  label: string;
  alias?: string;
  outputs: string[];
  config?: Record<string, unknown>;
  [key: string]: unknown;
}

export function NodaNode({ data, selected, id }: NodeProps) {
  const nodeData = data as unknown as NodaNodeData;
  const style = getCategoryStyle(nodeData.nodeType);
  const outputs = nodeData.outputs ?? ["success", "error"];
  const validationErrors = useEditorStore((s) => s.validationErrors);

  // Check if this node has validation errors
  const nodeErrors = validationErrors.filter(
    (e) => e.path?.includes(id) || e.message?.includes(id)
  );
  const hasError = nodeErrors.length > 0;

  return (
    <div
      className={`rounded-lg border-2 shadow-sm min-w-[160px] ${style.bg} ${
        hasError ? "border-red-400" : style.border
      } ${selected ? "ring-2 ring-blue-400 ring-offset-1" : ""}`}
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
          {hasError && (
            <span title={nodeErrors.map((e) => e.message).join("\n")}>
              <AlertCircle size={14} className="text-red-500" />
            </span>
          )}
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
          <div key={output} className="flex items-center justify-end gap-1.5 relative">
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
            {/* Invisible absolutely positioned handle for edge routing */}
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
    </div>
  );
}

function summarizeConfig(config: Record<string, unknown>): string {
  // Show the most relevant config value as a preview
  for (const key of ["query", "sql", "condition", "expression", "url", "template", "key", "channel", "topic"]) {
    if (key in config && typeof config[key] === "string") {
      const val = config[key] as string;
      return val.length > 40 ? val.slice(0, 37) + "..." : val;
    }
  }
  const keys = Object.keys(config);
  return keys.length <= 3 ? keys.join(", ") : `${keys.length} fields`;
}
