import { useEffect, useState, useCallback } from "react";
import { ViewHeader } from "@/components/layout/ViewHeader";
import { Field } from "@/components/ui/Field";
import { DetailHeader } from "@/components/ui/DetailHeader";
import { ConfigListDetail } from "@/components/ui/ConfigListDetail";
import * as api from "@/api/client";
import { useEditorStore } from "@/stores/editor";
import { showToast } from "@/utils/toast";
import type { VarInfo } from "@/types";

interface VarsData {
  [key: string]: string;
}

export function VarsView() {
  const files = useEditorStore((s) => s.files);
  const loadVars = useEditorStore((s) => s.loadVars);
  const vars = useEditorStore((s) => s.vars);

  const [allVars, setAllVars] = useState<VarInfo[]>([]);
  const [selectedName, setSelectedName] = useState<string | null>(null);
  const [editName, setEditName] = useState("");
  const [editValue, setEditValue] = useState("");
  const [isNew, setIsNew] = useState(false);
  const [saving, setSaving] = useState(false);
  const [filter, setFilter] = useState("");

  const reload = useCallback(async () => {
    await loadVars();
  }, [loadVars]);

  useEffect(() => {
    reload();
  }, [reload]);

  useEffect(() => {
    setAllVars(vars);
  }, [vars]);

  const selectVar = useCallback((v: VarInfo) => {
    setSelectedName(v.name);
    setEditName(v.name);
    setEditValue(v.value);
    setIsNew(false);
  }, []);

  const startNew = useCallback(() => {
    setSelectedName(null);
    setEditName("");
    setEditValue("");
    setIsNew(true);
  }, []);

  const handleSave = useCallback(async () => {
    if (!editName.trim()) return;
    setSaving(true);
    try {
      // Read current vars.json (or start fresh)
      let current: VarsData = {};
      if (files?.vars) {
        try {
          current = (await api.readFile(files.vars)) as VarsData;
        } catch {
          // File might not exist yet
        }
      }

      // If renaming, remove old key
      if (!isNew && selectedName && selectedName !== editName) {
        delete current[selectedName];
      }

      current[editName] = editValue;
      await api.writeFile("vars.json", current);
      showToast({ type: "success", message: `Variable "${editName}" saved` });
      setIsNew(false);
      setSelectedName(editName);
      await reload();
    } catch (err) {
      showToast({ type: "error", message: `Failed to save: ${err}` });
    } finally {
      setSaving(false);
    }
  }, [editName, editValue, isNew, selectedName, files?.vars, reload]);

  const handleDelete = useCallback(async () => {
    if (!selectedName) return;
    if (!confirm(`Delete variable "${selectedName}"?`)) return;
    try {
      let current: VarsData = {};
      if (files?.vars) {
        current = (await api.readFile(files.vars)) as VarsData;
      }
      delete current[selectedName];
      await api.writeFile("vars.json", current);
      showToast({ type: "success", message: `Variable deleted` });
      setSelectedName(null);
      setEditName("");
      setEditValue("");
      await reload();
    } catch (err) {
      showToast({ type: "error", message: `Failed to delete: ${err}` });
    }
  }, [selectedName, files?.vars, reload]);

  const selectedVar = allVars.find((v) => v.name === selectedName);
  const usages = isNew ? [] : (selectedVar?.usages ?? []);

  const filtered = filter
    ? allVars.filter(
        (v) =>
          v.name.toLowerCase().includes(filter.toLowerCase()) ||
          v.value.toLowerCase().includes(filter.toLowerCase()),
      )
    : allVars;

  return (
    <div className="flex-1 flex flex-col min-h-0">
      <ViewHeader
        title="Variables"
        subtitle="Shared values referenced across config files with {{ $var('NAME') }}"
      />
      <ConfigListDetail
        items={filtered}
        getKey={(v) => v.name}
        selectedKey={isNew ? null : selectedName}
        onSelect={(key) => {
          const v = allVars.find((x) => x.name === key);
          if (v) selectVar(v);
        }}
        renderItem={(v) => (
          <>
            <div className="text-sm font-medium text-gray-800 font-mono truncate">
              {v.name}
            </div>
            <div className="text-xs text-gray-400 font-mono truncate">
              {v.value || "—"}
            </div>
          </>
        )}
        title={`Variables (${allVars.length})`}
        onNew={startNew}
        emptyMessage={
          allVars.length === 0
            ? "No variables defined. Create vars.json to get started."
            : "No matching variables."
        }
        filter={{
          value: filter,
          onChange: setFilter,
          placeholder: "Filter variables...",
        }}
      >
        {isNew || selectedName ? (
          <div className="max-w-2xl space-y-5">
            <DetailHeader
              title={isNew ? "New Variable" : editName}
              isNew={isNew}
              saving={saving}
              onSave={handleSave}
              onDelete={handleDelete}
              saveDisabled={!editName.trim()}
            />

            <Field label="Name">
              <input
                type="text"
                value={editName}
                onChange={(e) =>
                  setEditName(
                    e.target.value.toUpperCase().replace(/[^A-Z0-9_]/g, "_"),
                  )
                }
                className="input-field font-mono"
                placeholder="e.g. MAIN_DB"
              />
            </Field>

            <Field label="Value">
              <input
                type="text"
                value={editValue}
                onChange={(e) => setEditValue(e.target.value)}
                className="input-field font-mono"
                placeholder="e.g. main-db"
              />
            </Field>

            <Field label="Reference">
              <div className="flex items-center gap-2">
                <code className="px-3 py-1.5 bg-gray-50 border border-gray-200 rounded text-sm font-mono text-gray-700 select-all">
                  {"{{ $var('" + editName + "') }}"}
                </code>
                <button
                  type="button"
                  onClick={() => {
                    navigator.clipboard.writeText(
                      `{{ $var('${editName}') }}`,
                    );
                    showToast({
                      type: "success",
                      message: "Copied to clipboard",
                    });
                  }}
                  className="text-xs text-blue-500 hover:text-blue-700"
                >
                  Copy
                </button>
              </div>
            </Field>

            {usages.length > 0 && (
              <div className="border-t border-gray-200 pt-4">
                <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-2">
                  Used In ({usages.length} file
                  {usages.length !== 1 ? "s" : ""})
                </h4>
                <ul className="space-y-1">
                  {usages.map((u) => (
                    <li key={u} className="text-sm text-gray-600 font-mono">
                      {u}
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </div>
        ) : (
          <div className="text-sm text-gray-400">
            Select a variable to edit or click "New" to create one.
          </div>
        )}
      </ConfigListDetail>
    </div>
  );
}
