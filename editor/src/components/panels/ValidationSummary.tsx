import { AlertTriangle } from "lucide-react";
import { useEditorStore } from "@/stores/editor";

export function ValidationSummary() {
  const errors = useEditorStore((s) => s.validationErrors);
  const selectNode = useEditorStore((s) => s.selectNode);

  if (errors.length === 0) return null;

  // Group errors by path (which may contain node IDs like "nodes[0].config")
  const grouped = new Map<string, typeof errors>();
  for (const err of errors) {
    const key =
      err.path.match(/nodes\[\d+\]/)?.[0] ??
      err.path.split(".")[0] ??
      "general";
    if (!grouped.has(key)) grouped.set(key, []);
    grouped.get(key)!.push(err);
  }

  return (
    <div className="border-t border-red-200 bg-red-50 px-4 py-2 max-h-40 overflow-y-auto">
      <div className="flex items-center gap-1.5 mb-1.5">
        <AlertTriangle size={14} className="text-red-500" />
        <span className="text-xs font-semibold text-red-700">
          {errors.length} validation error{errors.length !== 1 ? "s" : ""}
        </span>
      </div>
      {Array.from(grouped.entries()).map(([group, errs]) => (
        <div key={group} className="mb-1.5">
          <div className="text-xs font-medium text-red-600">{group}</div>
          {errs.map((err, i) => (
            <div
              key={i}
              className="text-xs text-red-500 ml-3 cursor-pointer hover:underline"
              onClick={() => {
                // Try to extract node ID from path like "nodes[0]"
                const match = err.path.match(/nodes\[(\d+)\]/);
                if (match) {
                  const activeWorkflow =
                    useEditorStore.getState().activeWorkflow;
                  const idx = parseInt(match[1], 10);
                  const node = activeWorkflow?.nodes[idx];
                  if (node) selectNode(node.id);
                }
              }}
            >
              {err.message}
            </div>
          ))}
        </div>
      ))}
    </div>
  );
}
