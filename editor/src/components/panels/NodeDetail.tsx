import { useEditorStore } from "@/stores/editor";

export function NodeDetail() {
  const selectedNodeId = useEditorStore((s) => s.selectedNodeId);
  const activeWorkflow = useEditorStore((s) => s.activeWorkflow);

  if (!selectedNodeId || !activeWorkflow) {
    return (
      <div className="p-4 text-sm text-gray-400">
        Select a node to view its configuration.
      </div>
    );
  }

  const node = activeWorkflow.nodes.find((n) => n.id === selectedNodeId);
  if (!node) {
    return (
      <div className="p-4 text-sm text-gray-400">Node not found.</div>
    );
  }

  return (
    <div className="p-4 overflow-y-auto">
      <div className="mb-3">
        <span className="text-xs font-medium text-gray-400 uppercase tracking-wider">
          Node
        </span>
        <h3 className="text-base font-semibold text-gray-900">
          {node.as ?? node.id}
        </h3>
        <p className="text-sm text-gray-500">{node.type}</p>
      </div>

      {node.services && Object.keys(node.services).length > 0 && (
        <div className="mb-3">
          <span className="text-xs font-medium text-gray-400 uppercase tracking-wider">
            Services
          </span>
          <div className="mt-1 space-y-1">
            {Object.entries(node.services).map(([slot, instance]) => (
              <div key={slot} className="text-sm">
                <span className="text-gray-500">{slot}:</span>{" "}
                <span className="text-gray-800">{instance}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {node.config && (
        <div>
          <span className="text-xs font-medium text-gray-400 uppercase tracking-wider">
            Config
          </span>
          <pre className="mt-1 p-2 bg-gray-50 rounded text-xs text-gray-700 overflow-x-auto whitespace-pre-wrap">
            {JSON.stringify(node.config, null, 2)}
          </pre>
        </div>
      )}
    </div>
  );
}
