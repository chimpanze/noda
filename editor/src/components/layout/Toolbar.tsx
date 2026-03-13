import { useRef, useState } from "react";
import { Undo2, Redo2, LayoutDashboard, Save, ShieldCheck, Download, Upload, FolderDown } from "lucide-react";
import { useEditorStore } from "@/stores/editor";
import { autoLayout } from "@/components/canvas/autoLayout";
import { exportWorkflow, importWorkflow, exportAllAsZip } from "@/utils/importExport";
import * as api from "@/api/client";

export function Toolbar() {
  const undo = useEditorStore((s) => s.undo);
  const redo = useEditorStore((s) => s.redo);
  const saveWorkflow = useEditorStore((s) => s.saveWorkflow);
  const validateAndSave = useEditorStore((s) => s.validateAndSave);
  const saveStatus = useEditorStore((s) => s.saveStatus);
  const autoSave = useEditorStore((s) => s.autoSave);
  const setAutoSave = useEditorStore((s) => s.setAutoSave);
  const dirtyFiles = useEditorStore((s) => s.dirtyFiles);
  const activeWorkflow = useEditorStore((s) => s.activeWorkflow);
  const activeWorkflowPath = useEditorStore((s) => s.activeWorkflowPath);
  const setWorkflow = useEditorStore((s) => s.setWorkflow);
  const validationErrors = useEditorStore((s) => s.validationErrors);
  const files = useEditorStore((s) => s.files);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [importError, setImportError] = useState<string | null>(null);

  const onAutoLayout = async () => {
    if (!activeWorkflow) return;
    const wf = await autoLayout(activeWorkflow);
    setWorkflow(wf);
  };

  const onExport = () => {
    if (!activeWorkflow || !activeWorkflowPath) return;
    const name = activeWorkflowPath.replace(/^.*\//, "").replace(/\.json$/, "");
    exportWorkflow(activeWorkflow, `${name}.json`);
  };

  const onImport = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    setImportError(null);
    try {
      const wf = await importWorkflow(file);
      setWorkflow(wf);
    } catch (err) {
      setImportError(err instanceof Error ? err.message : "Import failed");
    }
    // Reset input so same file can be re-imported
    e.target.value = "";
  };

  const onExportAll = () => {
    if (!files) return;
    const allPaths = [
      ...(files.workflows ?? []),
      ...(files.routes ?? []),
      ...(files.schemas ?? []),
      ...(files.workers ?? []),
      ...(files.schedules ?? []),
      ...(files.connections ?? []),
      ...(files.tests ?? []),
    ];
    exportAllAsZip(allPaths, api.readFile);
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
      <ToolButton
        icon={<ShieldCheck size={16} />}
        title="Validate & Save (Ctrl+Shift+S)"
        onClick={validateAndSave}
        disabled={!activeWorkflow}
      />
      <div className="w-px h-5 bg-gray-300 mx-1" />
      <ToolButton
        icon={<Download size={16} />}
        title="Export workflow JSON"
        onClick={onExport}
        disabled={!activeWorkflow}
      />
      <ToolButton
        icon={<Upload size={16} />}
        title="Import workflow JSON"
        onClick={() => fileInputRef.current?.click()}
        disabled={!activeWorkflow}
      />
      <ToolButton
        icon={<FolderDown size={16} />}
        title="Export all as ZIP"
        onClick={onExportAll}
        disabled={!files}
      />
      <input ref={fileInputRef} type="file" accept=".json" onChange={onImport} className="hidden" />

      <div className="flex-1" />

      {/* Unsaved indicator */}
      {!autoSave && dirtyFiles.size > 0 && (
        <span className="text-xs text-amber-600 bg-amber-50 px-2 py-0.5 rounded mr-2">
          Unsaved
        </span>
      )}

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

      {/* Auto-save toggle */}
      <label className="flex items-center gap-1 text-xs text-gray-500 mr-2 cursor-pointer select-none">
        <input
          type="checkbox"
          checked={autoSave}
          onChange={(e) => setAutoSave(e.target.checked)}
          className="accent-blue-500"
        />
        Auto-save
      </label>

      {/* Import error */}
      {importError && (
        <span className="text-xs text-red-500 mr-2 cursor-pointer" onClick={() => setImportError(null)} title="Click to dismiss">
          Import: {importError}
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
