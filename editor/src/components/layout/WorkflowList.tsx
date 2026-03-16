import { useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { useEditorStore } from "@/stores/editor";
import * as api from "@/api/client";

export function WorkflowList() {
  const files = useEditorStore((s) => s.files);
  const activeWorkflowPath = useEditorStore((s) => s.activeWorkflowPath);
  const setActiveWorkflow = useEditorStore((s) => s.setActiveWorkflow);
  const dirtyFiles = useEditorStore((s) => s.dirtyFiles);
  const loadFiles = useEditorStore((s) => s.loadFiles);
  const closeTab = useEditorStore((s) => s.closeTab);

  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState("");

  const handleDelete = async (path: string) => {
    const name = path.replace(/^workflows\//, "").replace(/\.json$/, "");
    if (!confirm(`Delete workflow "${name}"?`)) return;
    await api.deleteFile(path);
    closeTab(path);
    await loadFiles();
  };

  const workflows = files?.workflows ?? [];

  const handleCreate = async () => {
    const name = newName.trim();
    if (!name) return;
    const fileName = name.endsWith(".json") ? name : `${name}.json`;
    const path = `workflows/${fileName}`;
    const id = name.replace(/\.json$/, "");

    const content = {
      id,
      nodes: {},
      edges: [],
    };

    await api.writeFile(path, content);
    await loadFiles();
    setCreating(false);
    setNewName("");
    setActiveWorkflow(path);
  };

  return (
    <div className="border-r border-gray-200 w-52 overflow-y-auto shrink-0">
      <div className="px-3 py-2 flex items-center justify-between">
        <span className="text-xs font-medium text-gray-400 uppercase tracking-wider">
          Workflows
        </span>
        <button
          onClick={() => setCreating(true)}
          className="flex items-center gap-1 px-2 py-1 text-xs text-blue-600 hover:bg-blue-50 rounded"
        >
          <Plus size={14} />
          New
        </button>
      </div>

      {creating && (
        <div className="px-3 pb-2 flex gap-1">
          <input
            type="text"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") handleCreate();
              if (e.key === "Escape") {
                setCreating(false);
                setNewName("");
              }
            }}
            placeholder="workflow-name"
            className="flex-1 min-w-0 text-xs border border-gray-300 rounded px-2 py-1 focus:outline-none focus:ring-1 focus:ring-blue-400"
            autoFocus
          />
          <button
            onClick={handleCreate}
            className="text-xs px-2 py-1 bg-blue-600 text-white rounded hover:bg-blue-700"
          >
            Create
          </button>
        </div>
      )}

      {workflows.length === 0 && !creating && (
        <div className="px-3 py-2 text-sm text-gray-500">
          No workflows found.
        </div>
      )}

      {workflows.map((path) => {
        const name = path.replace(/^workflows\//, "").replace(/\.json$/, "");
        const isDirty = dirtyFiles.has(path);
        const isActive = activeWorkflowPath === path;
        return (
          <div
            key={path}
            className={`group flex items-center pr-1 ${
              isActive
                ? "bg-blue-50 text-blue-700 font-medium"
                : "text-gray-700 hover:bg-gray-50"
            }`}
          >
            <button
              onClick={() => setActiveWorkflow(path)}
              className="flex-1 text-left px-3 py-1.5 text-sm truncate flex items-center gap-1.5"
            >
              <span className="truncate">{name}</span>
              {isDirty && (
                <span
                  className="w-1.5 h-1.5 rounded-full bg-orange-400 shrink-0"
                  title="Unsaved changes"
                />
              )}
            </button>
            <button
              onClick={(e) => {
                e.stopPropagation();
                handleDelete(path);
              }}
              className="hidden group-hover:block p-1 text-gray-300 hover:text-red-500"
              title="Delete workflow"
            >
              <Trash2 size={12} />
            </button>
          </div>
        );
      })}
    </div>
  );
}
