import { useState } from "react";
import { Circle, CheckCircle, XCircle, Loader2, Wifi, WifiOff } from "lucide-react";
import { useTraceStore } from "@/stores/trace";
import { useEditorStore } from "@/stores/editor";
import type { Execution } from "@/types";

type Tab = "executions" | "detail";

export function TracePanel() {
  const executions = useTraceStore((s) => s.executions);
  const activeTraceId = useTraceStore((s) => s.activeTraceId);
  const setActiveTraceId = useTraceStore((s) => s.setActiveTraceId);
  const clearExecutions = useTraceStore((s) => s.clearExecutions);
  const connectionStatus = useTraceStore((s) => s.connectionStatus);
  const selectedNodeId = useEditorStore((s) => s.selectedNodeId);
  const selectNode = useEditorStore((s) => s.selectNode);

  const [tab, setTab] = useState<Tab>("executions");
  const [showErrorsOnly, setShowErrorsOnly] = useState(false);

  const activeExec = executions.find((e) => e.traceId === activeTraceId);

  return (
    <div className="h-full flex flex-col text-sm">
      {/* Header */}
      <div className="flex items-center justify-between px-3 py-1.5 border-b border-gray-200 bg-gray-50 shrink-0">
        <div className="flex items-center gap-3">
          <button
            onClick={() => setTab("executions")}
            className={`text-xs font-medium ${tab === "executions" ? "text-blue-600" : "text-gray-500 hover:text-gray-700"}`}
          >
            Executions ({executions.length})
          </button>
          <button
            onClick={() => setTab("detail")}
            className={`text-xs font-medium ${tab === "detail" ? "text-blue-600" : "text-gray-500 hover:text-gray-700"}`}
          >
            Node Detail
          </button>
        </div>
        <div className="flex items-center gap-2">
          <ConnectionIndicator status={connectionStatus} />
          {executions.length > 0 && (
            <button onClick={clearExecutions} className="text-xs text-gray-400 hover:text-gray-600">
              Clear
            </button>
          )}
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto">
        {tab === "executions" ? (
          <ExecutionList
            executions={executions}
            activeTraceId={activeTraceId}
            onSelect={setActiveTraceId}
          />
        ) : (
          <NodeDetailView
            execution={activeExec}
            selectedNodeId={selectedNodeId}
            onSelectNode={selectNode}
            showErrorsOnly={showErrorsOnly}
            setShowErrorsOnly={setShowErrorsOnly}
          />
        )}
      </div>
    </div>
  );
}

function ConnectionIndicator({ status }: { status: string }) {
  if (status === "connected") {
    return (
      <span className="flex items-center gap-1 text-xs text-green-600" title="Connected to trace WebSocket">
        <Wifi size={12} />
      </span>
    );
  }
  if (status === "connecting") {
    return (
      <span className="flex items-center gap-1 text-xs text-yellow-500" title="Connecting...">
        <Loader2 size={12} className="animate-spin" />
      </span>
    );
  }
  return (
    <span className="flex items-center gap-1 text-xs text-gray-400" title="Disconnected">
      <WifiOff size={12} />
    </span>
  );
}

function ExecutionList({
  executions,
  activeTraceId,
  onSelect,
}: {
  executions: Execution[];
  activeTraceId: string | null;
  onSelect: (id: string) => void;
}) {
  if (executions.length === 0) {
    return (
      <div className="p-4 text-gray-400 text-xs">
        No executions yet. Trigger a workflow to see live trace data.
      </div>
    );
  }

  return (
    <div className="divide-y divide-gray-100">
      {[...executions].reverse().map((exec) => (
        <button
          key={exec.traceId}
          onClick={() => onSelect(exec.traceId)}
          className={`w-full text-left px-3 py-2 hover:bg-gray-50 flex items-center gap-2 ${
            activeTraceId === exec.traceId ? "bg-blue-50" : ""
          }`}
        >
          <StatusIcon status={exec.status} />
          <div className="flex-1 min-w-0">
            <div className="text-xs font-medium text-gray-800 truncate">
              {exec.workflowId}
            </div>
            <div className="text-xs text-gray-400">
              {exec.traceId.slice(0, 8)}
              {exec.duration && ` · ${exec.duration}`}
            </div>
          </div>
          <span className="text-xs text-gray-400 shrink-0">
            {formatTime(exec.startedAt)}
          </span>
        </button>
      ))}
    </div>
  );
}

function NodeDetailView({
  execution,
  selectedNodeId,
  onSelectNode,
  showErrorsOnly,
  setShowErrorsOnly,
}: {
  execution: Execution | undefined;
  selectedNodeId: string | null;
  onSelectNode: (id: string) => void;
  showErrorsOnly: boolean;
  setShowErrorsOnly: (v: boolean) => void;
}) {
  if (!execution) {
    return (
      <div className="p-4 text-gray-400 text-xs">
        Select an execution to inspect node data.
      </div>
    );
  }

  // Build ordered list of nodes from events
  const nodeIds: string[] = [];
  for (const event of execution.events) {
    if (event.node_id && !nodeIds.includes(event.node_id)) {
      nodeIds.push(event.node_id);
    }
  }

  const filteredNodes = showErrorsOnly
    ? nodeIds.filter((id) => execution.nodeStates.get(id) === "failed")
    : nodeIds;

  return (
    <div>
      <div className="px-3 py-1.5 border-b border-gray-100 flex items-center gap-2">
        <label className="flex items-center gap-1 text-xs text-gray-500 cursor-pointer">
          <input
            type="checkbox"
            checked={showErrorsOnly}
            onChange={(e) => setShowErrorsOnly(e.target.checked)}
            className="rounded"
          />
          Errors only
        </label>
      </div>
      <div className="divide-y divide-gray-100">
        {filteredNodes.map((nodeId) => {
          const state = execution.nodeStates.get(nodeId) ?? "idle";
          const data = execution.nodeData.get(nodeId);
          const isSelected = selectedNodeId === nodeId;

          return (
            <div
              key={nodeId}
              className={`px-3 py-2 cursor-pointer hover:bg-gray-50 ${isSelected ? "bg-blue-50" : ""}`}
              onClick={() => onSelectNode(nodeId)}
            >
              <div className="flex items-center gap-2">
                <StatusIcon status={state === "completed" ? "completed" : state === "failed" ? "failed" : "running"} />
                <span className="text-xs font-medium text-gray-800">{nodeId}</span>
                {data?.duration && (
                  <span className="text-xs text-gray-400 ml-auto">{data.duration}</span>
                )}
              </div>
              {data?.output && (
                <div className="mt-1 text-xs text-gray-500">
                  Output: <span className="font-medium">{data.output}</span>
                </div>
              )}
              {data?.error && (
                <div className="mt-1 text-xs text-red-600">{data.error}</div>
              )}
              {data?.data != null && (
                <details className="mt-1">
                  <summary className="text-xs text-gray-400 cursor-pointer hover:text-gray-600">
                    Data
                  </summary>
                  <pre className="mt-1 p-2 bg-gray-50 rounded text-xs text-gray-600 overflow-x-auto whitespace-pre-wrap max-h-32">
                    {JSON.stringify(data.data, null, 2)}
                  </pre>
                </details>
              )}
            </div>
          );
        })}
        {filteredNodes.length === 0 && (
          <div className="p-3 text-xs text-gray-400">
            {showErrorsOnly ? "No errors in this execution." : "No node data yet."}
          </div>
        )}
      </div>
    </div>
  );
}

function StatusIcon({ status }: { status: string }) {
  switch (status) {
    case "completed":
      return <CheckCircle size={14} className="text-green-500 shrink-0" />;
    case "failed":
      return <XCircle size={14} className="text-red-500 shrink-0" />;
    case "running":
      return <Loader2 size={14} className="text-blue-500 animate-spin shrink-0" />;
    default:
      return <Circle size={14} className="text-gray-300 shrink-0" />;
  }
}

function formatTime(timestamp: string): string {
  try {
    const d = new Date(timestamp);
    return d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit", second: "2-digit" });
  } catch {
    return "";
  }
}
