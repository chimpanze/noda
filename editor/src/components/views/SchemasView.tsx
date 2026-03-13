import { useEffect, useState, useCallback, useMemo } from "react";
import { Plus, Trash2, Save, ChevronDown, ChevronRight } from "lucide-react";
import { ViewHeader } from "@/components/layout/ViewHeader";
import Editor from "@monaco-editor/react";
import * as api from "@/api/client";
import { useEditorStore } from "@/stores/editor";
import { showToast } from "@/components/panels/Toast";
import { SchemaPropertyEditor } from "@/components/widgets/SchemaPropertyEditor";
import { groupByFolder } from "@/components/widgets/SchemaSelect";
import type { SchemaInfo } from "@/types";

const SCHEMA_CATEGORIES = ["validation", "models", "params", "responses"];

type EditorMode = "visual" | "json";

export function SchemasView() {
  const loadFiles = useEditorStore((s) => s.loadFiles);

  const [schemas, setSchemas] = useState<SchemaInfo[]>([]);
  const [selected, setSelected] = useState<SchemaInfo | null>(null);
  const [editorValue, setEditorValue] = useState("");
  const [originalValue, setOriginalValue] = useState("");
  const [parseError, setParseError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState("");
  const [newCategory, setNewCategory] = useState("");
  const [customCategory, setCustomCategory] = useState("");
  const [editorMode, setEditorMode] = useState<EditorMode>("visual");
  const [collapsedFolders, setCollapsedFolders] = useState<Set<string>>(new Set());

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      const s = await api.listSchemas();
      setSchemas(s);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    reload();
  }, [reload]);

  const dirty = editorValue !== originalValue;

  const grouped = useMemo(() => groupByFolder(schemas), [schemas]);

  const selectSchema = useCallback((schema: SchemaInfo) => {
    const json = JSON.stringify(schema.schema, null, 2);
    setSelected(schema);
    setEditorValue(json);
    setOriginalValue(json);
    setParseError(null);
    setCreating(false);
  }, []);

  const onEditorChange = useCallback((value: string | undefined) => {
    const v = value ?? "";
    setEditorValue(v);
    try {
      JSON.parse(v);
      setParseError(null);
    } catch (e) {
      setParseError((e as Error).message);
    }
  }, []);

  const onVisualChange = useCallback(
    (content: Record<string, unknown>) => {
      const json = JSON.stringify(content, null, 2);
      setEditorValue(json);
      setParseError(null);
    },
    []
  );

  const handleSave = useCallback(async () => {
    if (!selected || parseError) return;
    setSaving(true);
    try {
      const content = JSON.parse(editorValue);
      await api.writeFile(selected.path, content);
      setOriginalValue(editorValue);
      showToast({ type: "success", message: `Schema "${selected.path}" saved` });
      await loadFiles();
      await reload();
    } catch (err) {
      showToast({ type: "error", message: `Failed to save: ${err}` });
    } finally {
      setSaving(false);
    }
  }, [selected, editorValue, parseError, loadFiles, reload]);

  const handleDelete = useCallback(async () => {
    if (!selected) return;
    if (!confirm(`Delete schema "${selected.path}"?`)) return;
    try {
      await api.deleteFile(selected.path);
      showToast({ type: "success", message: `Schema "${selected.path}" deleted` });
      setSelected(null);
      setEditorValue("");
      setOriginalValue("");
      await loadFiles();
      await reload();
    } catch (err) {
      showToast({ type: "error", message: `Failed to delete: ${err}` });
    }
  }, [selected, loadFiles, reload]);

  const handleCreate = useCallback(async () => {
    const name = newName.trim();
    if (!name) return;
    const folder = newCategory === "custom" ? customCategory.trim() : newCategory;
    const pathPrefix = folder ? `schemas/${folder}` : "schemas";
    const fileName = name.endsWith(".json") ? name : `${name}.json`;
    const path = `${pathPrefix}/${fileName}`;
    setSaving(true);
    try {
      const content = { [name.replace(/\.json$/, "")]: { type: "object", properties: {} } };
      await api.writeFile(path, content);
      showToast({ type: "success", message: `Schema "${path}" created` });
      setCreating(false);
      setNewName("");
      setNewCategory("");
      setCustomCategory("");
      await loadFiles();
      await reload();
      const json = JSON.stringify(content, null, 2);
      setSelected({ path, schema: content });
      setEditorValue(json);
      setOriginalValue(json);
    } catch (err) {
      showToast({ type: "error", message: `Failed to create: ${err}` });
    } finally {
      setSaving(false);
    }
  }, [newName, newCategory, customCategory, loadFiles, reload]);

  const toggleFolder = useCallback((folder: string) => {
    setCollapsedFolders((prev) => {
      const next = new Set(prev);
      if (next.has(folder)) next.delete(folder);
      else next.add(folder);
      return next;
    });
  }, []);

  // Parse editor value for visual mode
  const parsedContent = useMemo(() => {
    try {
      return JSON.parse(editorValue) as Record<string, unknown>;
    } catch {
      return null;
    }
  }, [editorValue]);

  if (loading) {
    return <div className="p-6 text-sm text-gray-400">Loading schemas...</div>;
  }

  return (
    <div className="flex-1 flex flex-col min-h-0">
      <ViewHeader title="Schemas" subtitle="Shared JSON Schema definitions for validation and migration generation" />
      <div className="flex-1 flex min-h-0">
      {/* Schema list */}
      <div className="w-64 border-r border-gray-200 overflow-y-auto">
        <div className="px-4 py-3 border-b border-gray-200 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-gray-800">
            Schemas ({schemas.length})
          </h2>
          <button
            onClick={() => setCreating(true)}
            className="flex items-center gap-1 px-2 py-1 text-xs text-blue-600 hover:bg-blue-50 rounded"
          >
            <Plus size={14} />
            New
          </button>
        </div>

        {creating && (
          <div className="px-4 py-2 border-b border-gray-200 bg-blue-50 space-y-1.5">
            <select
              value={newCategory}
              onChange={(e) => setNewCategory(e.target.value)}
              className="input-field text-xs w-full"
            >
              <option value="">General (root)</option>
              {SCHEMA_CATEGORIES.map((c) => (
                <option key={c} value={c}>{c}</option>
              ))}
              <option value="custom">Custom...</option>
            </select>
            {newCategory === "custom" && (
              <input
                type="text"
                value={customCategory}
                onChange={(e) => setCustomCategory(e.target.value)}
                className="input-field text-xs font-mono w-full"
                placeholder="folder/path"
              />
            )}
            <input
              type="text"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") handleCreate();
                if (e.key === "Escape") setCreating(false);
              }}
              className="input-field text-xs font-mono w-full"
              placeholder="SchemaName"
              autoFocus
            />
            <div className="flex gap-1">
              <button
                onClick={handleCreate}
                disabled={!newName.trim()}
                className="text-xs text-blue-600 hover:text-blue-800 disabled:opacity-30"
              >
                Create
              </button>
              <button
                onClick={() => { setCreating(false); setNewName(""); setNewCategory(""); setCustomCategory(""); }}
                className="text-xs text-gray-500 hover:text-gray-700"
              >
                Cancel
              </button>
            </div>
          </div>
        )}

        <div>
          {[...grouped.entries()].map(([folder, items]) => {
            const label = folder || "General";
            const isCollapsed = collapsedFolders.has(folder);

            return (
              <div key={folder}>
                <button
                  onClick={() => toggleFolder(folder)}
                  className="w-full flex items-center gap-1 px-4 py-1.5 text-[10px] font-medium text-gray-400 uppercase tracking-wider hover:bg-gray-50"
                >
                  {isCollapsed ? <ChevronRight size={10} /> : <ChevronDown size={10} />}
                  {label}
                  <span className="ml-auto text-gray-300">{items.length}</span>
                </button>
                {!isCollapsed && (
                  <div className="divide-y divide-gray-100">
                    {items.map((schema) => {
                      const displayName = schema.path
                        .replace(/^schemas\//, "")
                        .replace(/\.json$/, "");
                      const shortName = folder
                        ? displayName.replace(new RegExp(`^${folder.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}\\/`), "")
                        : displayName;

                      return (
                        <button
                          key={schema.path}
                          onClick={() => selectSchema(schema)}
                          className={`w-full text-left px-4 py-2.5 hover:bg-gray-50 ${
                            selected?.path === schema.path ? "bg-blue-50" : ""
                          }`}
                        >
                          <div className="text-sm font-medium text-gray-800 truncate">
                            {shortName}
                          </div>
                          <div className="text-xs text-gray-400 truncate">{schema.path}</div>
                        </button>
                      );
                    })}
                  </div>
                )}
              </div>
            );
          })}
          {schemas.length === 0 && (
            <div className="p-4 text-sm text-gray-400">No schemas defined.</div>
          )}
        </div>
      </div>

      {/* Schema editor */}
      <div className="flex-1 flex flex-col min-h-0">
        {selected ? (
          <>
            <div className="px-4 py-2 border-b border-gray-200 flex items-center justify-between bg-gray-50 shrink-0">
              <div className="flex items-center gap-2">
                <span className="text-sm font-medium text-gray-800">
                  {selected.path}
                </span>
                {dirty && (
                  <span className="text-xs text-yellow-600 font-medium">
                    (unsaved)
                  </span>
                )}
              </div>
              <div className="flex items-center gap-2">
                <div className="flex items-center gap-0.5 bg-gray-100 rounded p-0.5">
                  <button
                    onClick={() => setEditorMode("visual")}
                    className={`px-2 py-0.5 text-xs rounded ${
                      editorMode === "visual"
                        ? "bg-white text-gray-800 shadow-sm font-medium"
                        : "text-gray-500 hover:text-gray-700"
                    }`}
                  >
                    Visual
                  </button>
                  <button
                    onClick={() => setEditorMode("json")}
                    className={`px-2 py-0.5 text-xs rounded ${
                      editorMode === "json"
                        ? "bg-white text-gray-800 shadow-sm font-medium"
                        : "text-gray-500 hover:text-gray-700"
                    }`}
                  >
                    JSON
                  </button>
                </div>
                {parseError ? (
                  <span className="text-xs text-red-500 truncate max-w-xs">
                    Invalid JSON: {parseError}
                  </span>
                ) : (
                  <span className="text-xs text-green-600">Valid</span>
                )}
                <button
                  onClick={handleSave}
                  disabled={!dirty || !!parseError || saving}
                  className="flex items-center gap-1 px-2 py-1 text-xs text-white bg-blue-500 rounded hover:bg-blue-600 disabled:opacity-40 disabled:cursor-not-allowed"
                >
                  <Save size={12} />
                  {saving ? "Saving..." : "Save"}
                </button>
                <button
                  onClick={handleDelete}
                  className="flex items-center gap-1 px-2 py-1 text-xs text-red-600 border border-red-300 rounded hover:bg-red-50"
                >
                  <Trash2 size={12} />
                </button>
              </div>
            </div>
            <div className="flex-1 min-h-0 overflow-auto">
              {editorMode === "visual" && parsedContent ? (
                <SchemaPropertyEditor
                  content={parsedContent as Record<string, Record<string, unknown>>}
                  onChange={onVisualChange}
                />
              ) : (
                <Editor
                  height="100%"
                  language="json"
                  value={editorValue}
                  onChange={onEditorChange}
                  options={{
                    minimap: { enabled: false },
                    fontSize: 13,
                    scrollBeyondLastLine: false,
                    wordWrap: "on",
                    formatOnPaste: true,
                  }}
                />
              )}
            </div>
          </>
        ) : (
          <div className="flex-1 flex items-center justify-center text-sm text-gray-400">
            Select a schema to edit or click "New" to create one.
          </div>
        )}
      </div>
      </div>
    </div>
  );
}
