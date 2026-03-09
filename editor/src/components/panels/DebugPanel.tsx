import { useEditorStore } from "@/stores/editor";

export function DebugPanel() {
  const traces = useEditorStore((s) => s.traces);
  const clearTraces = useEditorStore((s) => s.clearTraces);

  return (
    <div className="h-full flex flex-col">
      <div className="flex items-center justify-between px-3 py-1.5 border-b border-gray-200 bg-gray-50">
        <span className="text-xs font-medium text-gray-500 uppercase tracking-wider">
          Trace Log
        </span>
        {traces.length > 0 && (
          <button
            onClick={clearTraces}
            className="text-xs text-gray-400 hover:text-gray-600"
          >
            Clear
          </button>
        )}
      </div>
      <div className="flex-1 overflow-y-auto p-2 font-mono text-xs">
        {traces.length === 0 ? (
          <div className="text-gray-400 p-2">
            No trace events yet. Execute a workflow to see live trace data.
          </div>
        ) : (
          traces.map((event, i) => (
            <div key={i} className="py-0.5 text-gray-600">
              <span className="text-gray-400">[{event.type}]</span>{" "}
              {event.workflow_id}
              {event.trace_id && (
                <span className="text-gray-400"> #{event.trace_id.slice(0, 8)}</span>
              )}
            </div>
          ))
        )}
      </div>
    </div>
  );
}
