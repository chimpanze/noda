import { useEditorStore } from "@/stores/editor";

export function WorkflowList() {
  const files = useEditorStore((s) => s.files);
  const activeWorkflowPath = useEditorStore((s) => s.activeWorkflowPath);
  const setActiveWorkflow = useEditorStore((s) => s.setActiveWorkflow);

  const workflows = files?.workflows ?? [];

  if (workflows.length === 0) {
    return (
      <div className="p-4 text-sm text-gray-500">No workflows found.</div>
    );
  }

  return (
    <div className="border-r border-gray-200 w-52 overflow-y-auto shrink-0">
      <div className="px-3 py-2 text-xs font-medium text-gray-400 uppercase tracking-wider">
        Workflows
      </div>
      {workflows.map((path) => {
        const name = path.replace(/^workflows\//, "").replace(/\.json$/, "");
        return (
          <button
            key={path}
            onClick={() => setActiveWorkflow(path)}
            className={`w-full text-left px-3 py-1.5 text-sm truncate transition-colors ${
              activeWorkflowPath === path
                ? "bg-blue-50 text-blue-700 font-medium"
                : "text-gray-700 hover:bg-gray-50"
            }`}
          >
            {name}
          </button>
        );
      })}
    </div>
  );
}
