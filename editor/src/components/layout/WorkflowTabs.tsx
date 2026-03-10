import { X } from "lucide-react";
import { useEditorStore } from "@/stores/editor";

export function WorkflowTabs() {
  const openTabs = useEditorStore((s) => s.openTabs);
  const activeWorkflowPath = useEditorStore((s) => s.activeWorkflowPath);
  const setActiveWorkflow = useEditorStore((s) => s.setActiveWorkflow);
  const closeTab = useEditorStore((s) => s.closeTab);
  const dirtyFiles = useEditorStore((s) => s.dirtyFiles);

  if (openTabs.length <= 1) return null;

  return (
    <div className="flex items-center border-b border-gray-200 bg-gray-50 overflow-x-auto shrink-0">
      {openTabs.map((path) => {
        const name = path.replace(/^workflows\//, "").replace(/\.json$/, "");
        const isActive = path === activeWorkflowPath;
        const isDirty = dirtyFiles.has(path);

        return (
          <div
            key={path}
            className={`flex items-center gap-1 px-3 py-1.5 text-xs border-r border-gray-200 cursor-pointer shrink-0 ${
              isActive
                ? "bg-white text-blue-700 font-medium border-b-2 border-b-blue-500"
                : "text-gray-600 hover:bg-gray-100"
            }`}
            onClick={() => setActiveWorkflow(path)}
          >
            <span className="truncate max-w-[120px]">{name}</span>
            {isDirty && (
              <span className="w-1.5 h-1.5 rounded-full bg-orange-400 shrink-0" />
            )}
            <button
              onClick={(e) => {
                e.stopPropagation();
                closeTab(path);
              }}
              className="ml-1 p-0.5 rounded hover:bg-gray-200 text-gray-400 hover:text-gray-600"
            >
              <X size={10} />
            </button>
          </div>
        );
      })}
    </div>
  );
}
