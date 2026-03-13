import { useCallback } from "react";
import { useEditorStore } from "@/stores/editor";
import type { RetryConfig } from "@/types";

export function EdgeConfigPanel() {
  const selectedEdgeIndex = useEditorStore((s) => s.selectedEdgeIndex);
  const activeWorkflow = useEditorStore((s) => s.activeWorkflow);
  const updateEdgeRetry = useEditorStore((s) => s.updateEdgeRetry);
  const removeEdge = useEditorStore((s) => s.removeEdge);
  const deselectAll = useEditorStore((s) => s.deselectAll);

  const edge =
    selectedEdgeIndex !== null
      ? activeWorkflow?.edges[selectedEdgeIndex]
      : undefined;
  const hasRetry = !!edge?.retry;

  const toggleRetry = useCallback(() => {
    if (selectedEdgeIndex === null) return;
    if (hasRetry) {
      updateEdgeRetry(selectedEdgeIndex, undefined);
    } else {
      updateEdgeRetry(selectedEdgeIndex, { attempts: 3, delay: "1s" });
    }
  }, [hasRetry, selectedEdgeIndex, updateEdgeRetry]);

  const updateRetryField = useCallback(
    (field: keyof RetryConfig, value: string | number | undefined) => {
      if (selectedEdgeIndex === null || !edge?.retry) return;
      const next = { ...edge.retry, [field]: value };
      if (value === undefined || value === "") delete next[field];
      updateEdgeRetry(selectedEdgeIndex, next);
    },
    [edge, selectedEdgeIndex, updateEdgeRetry],
  );

  const handleDelete = useCallback(() => {
    if (!edge) return;
    removeEdge(edge.from, edge.output, edge.to);
    deselectAll();
  }, [edge, removeEdge, deselectAll]);

  if (selectedEdgeIndex === null || !activeWorkflow || !edge) return null;

  return (
    <div className="flex flex-col h-full overflow-hidden">
      {/* Header */}
      <div className="px-4 py-3 border-b border-gray-200 shrink-0">
        <div className="text-xs font-mono text-gray-400">Edge</div>
        <div className="text-sm font-semibold text-gray-900">
          {edge.from} &rarr; {edge.to}
        </div>
        <div className="text-xs text-gray-500 mt-0.5">
          output: {edge.output}
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {/* Retry config */}
        <div>
          <div className="flex items-center justify-between mb-2">
            <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider">
              Retry
            </h4>
            <button
              type="button"
              role="switch"
              aria-checked={hasRetry}
              onClick={toggleRetry}
              className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${
                hasRetry ? "bg-blue-500" : "bg-gray-300"
              } cursor-pointer`}
            >
              <span
                className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white transition-transform ${
                  hasRetry ? "translate-x-4" : "translate-x-0.5"
                }`}
              />
            </button>
          </div>

          {hasRetry && edge.retry && (
            <div className="space-y-2">
              <div>
                <label className="text-sm font-medium text-gray-700 block mb-1">
                  Attempts
                </label>
                <input
                  type="number"
                  min={1}
                  step={1}
                  value={edge.retry.attempts}
                  onChange={(e) =>
                    updateRetryField(
                      "attempts",
                      parseInt(e.target.value, 10) || 1,
                    )
                  }
                  className="w-full px-3 py-1.5 text-sm border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-400 focus:border-transparent"
                />
              </div>
              <div>
                <label className="text-sm font-medium text-gray-700 block mb-1">
                  Delay
                </label>
                <input
                  type="text"
                  value={edge.retry.delay}
                  onChange={(e) => updateRetryField("delay", e.target.value)}
                  className="w-full px-3 py-1.5 text-sm border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-400 focus:border-transparent"
                  placeholder="e.g. 1s, 500ms"
                />
              </div>
              <div>
                <label className="text-sm font-medium text-gray-700 block mb-1">
                  Backoff
                </label>
                <select
                  value={edge.retry.backoff ?? ""}
                  onChange={(e) =>
                    updateRetryField("backoff", e.target.value || undefined)
                  }
                  className="w-full px-3 py-1.5 text-sm border border-gray-300 rounded bg-white focus:outline-none focus:ring-2 focus:ring-blue-400 focus:border-transparent"
                >
                  <option value="">None</option>
                  <option value="linear">Linear</option>
                  <option value="exponential">Exponential</option>
                </select>
              </div>
            </div>
          )}
        </div>

        {/* Delete edge */}
        <button
          type="button"
          onClick={handleDelete}
          className="w-full px-3 py-1.5 text-sm text-red-600 border border-red-300 rounded hover:bg-red-50"
        >
          Delete edge
        </button>
      </div>
    </div>
  );
}
