import { Undo2, Redo2, LayoutDashboard, Save } from "lucide-react";
import { useEditorStore } from "@/stores/editor";
import { autoLayout } from "@/components/canvas/autoLayout";

export function Toolbar() {
  const undo = useEditorStore((s) => s.undo);
  const redo = useEditorStore((s) => s.redo);
  const saveWorkflow = useEditorStore((s) => s.saveWorkflow);
  const saveStatus = useEditorStore((s) => s.saveStatus);
  const activeWorkflow = useEditorStore((s) => s.activeWorkflow);
  const setWorkflow = useEditorStore((s) => s.setWorkflow);
  const validationErrors = useEditorStore((s) => s.validationErrors);

  const onAutoLayout = async () => {
    if (!activeWorkflow) return;
    const wf = await autoLayout(activeWorkflow);
    setWorkflow(wf);
  };

  return (
    <div className="flex items-center gap-1 px-2 py-1 border-b border-gray-200 bg-gray-50 shrink-0">
      <ToolButton icon={<Undo2 size={16} />} title="Undo (Ctrl+Z)" onClick={undo} />
      <ToolButton icon={<Redo2 size={16} />} title="Redo (Ctrl+Shift+Z)" onClick={redo} />
      <div className="w-px h-5 bg-gray-300 mx-1" />
      <ToolButton
        icon={<LayoutDashboard size={16} />}
        title="Auto-layout (Ctrl+Shift+F)"
        onClick={onAutoLayout}
        disabled={!activeWorkflow}
      />
      <ToolButton
        icon={<Save size={16} />}
        title="Save (Ctrl+S)"
        onClick={saveWorkflow}
        disabled={!activeWorkflow}
      />

      <div className="flex-1" />

      {/* Save status */}
      {saveStatus !== "idle" && (
        <span
          className={`text-xs mr-2 ${
            saveStatus === "saving"
              ? "text-blue-500"
              : saveStatus === "saved"
                ? "text-green-500"
                : "text-red-500"
          }`}
        >
          {saveStatus === "saving" ? "Saving..." : saveStatus === "saved" ? "Saved" : "Error"}
        </span>
      )}

      {/* Validation status */}
      {validationErrors.length > 0 ? (
        <span className="text-xs text-red-600 bg-red-50 px-2 py-0.5 rounded">
          {validationErrors.length} error{validationErrors.length !== 1 ? "s" : ""}
        </span>
      ) : (
        activeWorkflow && (
          <span className="text-xs text-green-600 bg-green-50 px-2 py-0.5 rounded">
            Valid
          </span>
        )
      )}
    </div>
  );
}

function ToolButton({
  icon,
  title,
  onClick,
  disabled,
}: {
  icon: React.ReactNode;
  title: string;
  onClick: () => void;
  disabled?: boolean;
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      title={title}
      className="p-1.5 rounded text-gray-500 hover:bg-gray-200 hover:text-gray-800 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
    >
      {icon}
    </button>
  );
}
