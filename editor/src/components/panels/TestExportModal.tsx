import { useState, useEffect, useRef } from "react";
import { X, Save } from "lucide-react";
import type { Execution } from "../../types";
import { generateTestCase, testCaseToJSON, CORE_NODE_TYPES } from "../../lib/testGenerator";
import { useEditorStore } from "../../stores/editor";
import * as api from "../../api/client";

interface TestExportModalProps {
  execution: Execution;
  onClose: () => void;
}

export function TestExportModal({ execution, onClose }: TestExportModalProps) {
  const activeWorkflowPath = useEditorStore((s) => s.activeWorkflowPath);
  const [json, setJson] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  // Derive test file path from workflow path
  const testFilePath = (() => {
    const wfPath = activeWorkflowPath ?? `workflows/${execution.workflowId}.json`;
    // e.g. "workflows/create-user.json" → "tests/test-create-user.json"
    const filename = wfPath.split("/").pop() ?? `${execution.workflowId}.json`;
    const base = filename.replace(/\.json$/, "");
    return `tests/test-${base}.json`;
  })();

  // Generate initial JSON on mount
  useEffect(() => {
    const testCase = generateTestCase(execution, CORE_NODE_TYPES);
    // Start without an existing suite — we'll merge on save
    const initialJson = testCaseToJSON(testCase, execution.workflowId);
    setJson(initialJson);
  }, [execution]);

  // Close on Escape
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  const handleSave = async () => {
    setSaving(true);
    setError(null);
    try {
      let parsed: unknown;
      try {
        parsed = JSON.parse(json);
      } catch {
        setError("Invalid JSON — please fix the syntax before saving.");
        setSaving(false);
        return;
      }

      // Try to read existing test file and append the new test case
      let finalContent: unknown = parsed;
      try {
        const existing = await api.readFile(testFilePath);
        if (
          existing &&
          typeof existing === "object" &&
          !Array.isArray(existing) &&
          Array.isArray((existing as Record<string, unknown>).tests)
        ) {
          const existingSuite = existing as { id: string; workflow: string; tests: unknown[] };
          const newSuite = parsed as { id: string; workflow: string; tests: unknown[] };
          finalContent = {
            ...existingSuite,
            tests: [...existingSuite.tests, ...newSuite.tests],
          };
        }
      } catch {
        // File doesn't exist yet — use the generated content as-is
      }

      await api.writeFile(testFilePath, finalContent);
      setSaved(true);
      setTimeout(() => onClose(), 800);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save test file.");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      onClick={onClose}
    >
      <div
        className="bg-white rounded-lg shadow-xl w-[560px] max-h-[80vh] flex flex-col"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-gray-200">
          <div>
            <h2 className="text-sm font-semibold text-gray-800">Export as Test</h2>
            <p className="text-xs text-gray-500 mt-0.5">
              Saving to <code className="bg-gray-100 px-1 rounded">{testFilePath}</code>
            </p>
          </div>
          <button
            onClick={onClose}
            className="p-1 text-gray-400 hover:text-gray-600 rounded"
            aria-label="Close"
          >
            <X size={16} />
          </button>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-auto p-4">
          <textarea
            ref={textareaRef}
            value={json}
            onChange={(e) => setJson(e.target.value)}
            className="w-full h-64 font-mono text-xs border border-gray-200 rounded p-2 resize-none focus:outline-none focus:ring-1 focus:ring-blue-400"
            spellCheck={false}
          />
          {error && (
            <p className="mt-2 text-xs text-red-600">{error}</p>
          )}
          {saved && (
            <p className="mt-2 text-xs text-green-600">Saved successfully.</p>
          )}
        </div>

        {/* Footer */}
        <div className="flex justify-end gap-2 px-4 py-3 border-t border-gray-200">
          <button
            onClick={onClose}
            className="px-3 py-1.5 text-xs text-gray-600 hover:text-gray-800 border border-gray-200 rounded hover:bg-gray-50"
          >
            Cancel
          </button>
          <button
            onClick={handleSave}
            disabled={saving || saved}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-white bg-blue-600 rounded hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <Save size={12} />
            {saving ? "Saving..." : saved ? "Saved" : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}
