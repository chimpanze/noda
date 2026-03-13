import { useEditorStore } from "@/stores/editor";

export function WorkflowPropertiesPanel() {
  const activeWorkflowPath = useEditorStore((s) => s.activeWorkflowPath);
  const activeWorkflow = useEditorStore((s) => s.activeWorkflow);
  const updateWorkflowMeta = useEditorStore((s) => s.updateWorkflowMeta);

  if (!activeWorkflow || !activeWorkflowPath) return null;

  return (
    <div className="p-4 space-y-4">
      <h3 className="text-sm font-semibold text-gray-800">Workflow Properties</h3>

      <Field label="Path">
        <div className="text-sm text-gray-600 font-mono bg-gray-50 rounded px-2 py-1.5 border border-gray-200">
          {activeWorkflowPath}
        </div>
      </Field>

      <Field label="Description">
        <textarea
          value={activeWorkflow.description ?? ""}
          onChange={(e) => updateWorkflowMeta({ description: e.target.value || undefined })}
          className="input-field resize-y min-h-[60px]"
          placeholder="Describe this workflow..."
          rows={3}
        />
      </Field>

      <Field label="Version">
        <input
          type="text"
          value={activeWorkflow.version ?? ""}
          onChange={(e) => updateWorkflowMeta({ version: e.target.value || undefined })}
          className="input-field font-mono"
          placeholder="e.g. 1.0.0"
        />
      </Field>

      <div className="border-t border-gray-200 pt-3">
        <div className="grid grid-cols-2 gap-3 text-sm">
          <div>
            <span className="text-xs text-gray-400 uppercase block">Nodes</span>
            <span className="text-gray-700 font-medium">{activeWorkflow.nodes.length}</span>
          </div>
          <div>
            <span className="text-xs text-gray-400 uppercase block">Edges</span>
            <span className="text-gray-700 font-medium">{activeWorkflow.edges.length}</span>
          </div>
        </div>
      </div>
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <label className="text-xs font-medium text-gray-400 uppercase block mb-1">
        {label}
      </label>
      {children}
    </div>
  );
}
