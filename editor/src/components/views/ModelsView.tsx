import { useEffect, useState, useCallback, useMemo } from "react";
import { Plus, Trash2, Save, Database, Wand2 } from "lucide-react";
import { ReactFlowProvider } from "@xyflow/react";
import { ViewHeader } from "@/components/layout/ViewHeader";
import { ModelFormPanel } from "./ModelFormPanel";
import { ERDiagramTab } from "./ERDiagramTab";
import Editor from "@monaco-editor/react";
import * as api from "@/api/client";
import { useEditorStore } from "@/stores/editor";
import { showToast } from "@/utils/toast";
import type { ModelInfo, ModelDefinition } from "@/types";
import { MigrationPreviewDialog } from "./MigrationPreviewDialog";
import { CRUDGenerationDialog } from "./CRUDGenerationDialog";

type EditorTab = "editor" | "diagram" | "json";

const EMPTY_MODEL: ModelDefinition = {
  table: "",
  columns: {
    id: {
      type: "uuid",
      primary_key: true,
      default: "gen_random_uuid()",
      order: 0,
    },
  },
  relations: {},
  indexes: [],
  timestamps: true,
  soft_delete: false,
};

export function ModelsView() {
  const loadFiles = useEditorStore((s) => s.loadFiles);

  const [models, setModels] = useState<ModelInfo[]>([]);
  const [selected, setSelected] = useState<ModelInfo | null>(null);
  const [editorValue, setEditorValue] = useState("");
  const [originalValue, setOriginalValue] = useState("");
  const [activeTab, setActiveTab] = useState<EditorTab>("editor");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState("");

  // CRUD generation dialog state
  const [showCRUD, setShowCRUD] = useState(false);
  const [crudOps, setCrudOps] = useState<string[]>([
    "create",
    "list",
    "get",
    "update",
    "delete",
  ]);
  const [crudArtifacts, setCrudArtifacts] = useState<string[]>([
    "routes",
    "workflows",
    "schemas",
  ]);
  const [crudService, setCrudService] = useState("db");
  const [crudBasePath, setCrudBasePath] = useState("");
  const [crudScopeCol, setCrudScopeCol] = useState("");
  const [crudScopeParam, setCrudScopeParam] = useState("");
  const [crudPreview, setCrudPreview] = useState<Record<
    string,
    unknown
  > | null>(null);

  // Migration dialog state
  const [showMigration, setShowMigration] = useState(false);
  const [migrationUp, setMigrationUp] = useState("");
  const [migrationDown, setMigrationDown] = useState("");

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      const m = await api.listModels();
      setModels(m);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    reload();
  }, [reload]);

  const dirty = editorValue !== originalValue;

  const allTables = useMemo(
    () =>
      models
        .map((m) => m.model.table)
        .filter(Boolean)
        .sort(),
    [models],
  );

  const selectModel = useCallback((m: ModelInfo) => {
    // Assign order to columns that lack it (preserves iteration order from JSON load)
    const needsOrder = Object.values(m.model.columns).some(
      (c) => c.order == null,
    );
    if (needsOrder) {
      let i = 0;
      for (const col of Object.values(m.model.columns)) {
        if (col.order == null) col.order = i;
        i++;
      }
    }
    const json = JSON.stringify(m.model, null, 2);
    setSelected(m);
    setEditorValue(json);
    setOriginalValue(json);
    setCreating(false);
    setShowCRUD(false);
    setShowMigration(false);
  }, []);

  const parsedModel = useMemo((): ModelDefinition | null => {
    try {
      return JSON.parse(editorValue) as ModelDefinition;
    } catch {
      return null;
    }
  }, [editorValue]);

  const onFormChange = useCallback((model: ModelDefinition) => {
    setEditorValue(JSON.stringify(model, null, 2));
  }, []);

  const onJsonChange = useCallback((value: string | undefined) => {
    setEditorValue(value ?? "");
  }, []);

  const handleSave = useCallback(async () => {
    if (!selected || !parsedModel) return;
    setSaving(true);
    try {
      await api.writeFile(selected.path, parsedModel);
      setOriginalValue(editorValue);
      showToast({ type: "success", message: `Model "${selected.path}" saved` });
      await loadFiles();
      await reload();
    } catch (err) {
      showToast({ type: "error", message: `Failed to save: ${err}` });
    } finally {
      setSaving(false);
    }
  }, [selected, parsedModel, editorValue, loadFiles, reload]);

  const handleDelete = useCallback(async () => {
    if (!selected) return;
    if (!confirm(`Delete model "${selected.path}"?`)) return;
    try {
      await api.deleteFile(selected.path);
      showToast({ type: "success", message: `Model deleted` });
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
    const fileName = name.endsWith(".json") ? name : `${name}.json`;
    const path = `models/${fileName}`;
    const tableName = name.replace(/\.json$/, "");
    const content: ModelDefinition = { ...EMPTY_MODEL, table: tableName };
    setSaving(true);
    try {
      await api.writeFile(path, content);
      showToast({ type: "success", message: `Model "${path}" created` });
      setCreating(false);
      setNewName("");
      await loadFiles();
      await reload();
      const json = JSON.stringify(content, null, 2);
      setSelected({ path, model: content });
      setEditorValue(json);
      setOriginalValue(json);
    } catch (err) {
      showToast({ type: "error", message: `Failed to create: ${err}` });
    } finally {
      setSaving(false);
    }
  }, [newName, loadFiles, reload]);

  const handleGenerateMigration = useCallback(
    async (confirm = false) => {
      try {
        const result = await api.generateMigration(confirm);
        if (result.status === "no_changes") {
          showToast({ type: "info", message: "No model changes to migrate" });
          setShowMigration(false);
          return;
        }
        if (result.status === "preview") {
          setMigrationUp(result.up);
          setMigrationDown(result.down);
          setShowMigration(true);
          return;
        }
        // confirmed & created
        showToast({
          type: "success",
          message: `Migration created: ${result.up_path}`,
        });
        setShowMigration(false);
        await loadFiles();
      } catch (err) {
        showToast({ type: "error", message: `Migration failed: ${err}` });
      }
    },
    [loadFiles],
  );

  const handleGenerateCRUD = useCallback(
    async (confirm = false) => {
      if (!selected) return;
      try {
        const result = await api.generateCRUD({
          model: selected.path,
          confirm,
          service: crudService,
          base_path: crudBasePath || undefined,
          operations: crudOps,
          artifacts: crudArtifacts,
          scope_column: crudScopeCol || undefined,
          scope_param: crudScopeParam || undefined,
        });
        if (result.status === "preview") {
          setCrudPreview(result.files);
          return;
        }
        showToast({
          type: "success",
          message: `CRUD files created (${Object.keys(result.files).length} files)`,
        });
        setShowCRUD(false);
        setCrudPreview(null);
        await loadFiles();
        await reload();
      } catch (err) {
        showToast({ type: "error", message: `CRUD generation failed: ${err}` });
      }
    },
    [
      selected,
      crudService,
      crudBasePath,
      crudOps,
      crudArtifacts,
      crudScopeCol,
      crudScopeParam,
      loadFiles,
      reload,
    ],
  );

  const toggleCrudOp = (op: string) => {
    setCrudOps((prev) =>
      prev.includes(op) ? prev.filter((o) => o !== op) : [...prev, op],
    );
  };

  const toggleCrudArtifact = (a: string) => {
    setCrudArtifacts((prev) =>
      prev.includes(a) ? prev.filter((x) => x !== a) : [...prev, a],
    );
  };

  if (loading) {
    return <div className="p-6 text-sm text-gray-400">Loading models...</div>;
  }

  return (
    <div className="flex-1 flex flex-col min-h-0">
      <ViewHeader
        title="Models"
        subtitle="Database table definitions, ER diagram, and CRUD generation"
      />
      <div className="flex-1 flex min-h-0">
        {/* Model list sidebar */}
        <div className="w-56 border-r border-gray-200 overflow-y-auto">
          <div className="px-4 py-3 border-b border-gray-200 flex items-center justify-between">
            <h2 className="text-sm font-semibold text-gray-800">
              Models ({models.length})
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
              <input
                type="text"
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") handleCreate();
                  if (e.key === "Escape") setCreating(false);
                }}
                className="input-field text-xs font-mono w-full"
                placeholder="table_name"
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
                  onClick={() => {
                    setCreating(false);
                    setNewName("");
                  }}
                  className="text-xs text-gray-500 hover:text-gray-700"
                >
                  Cancel
                </button>
              </div>
            </div>
          )}

          <div className="divide-y divide-gray-100">
            {models.map((m) => (
              <button
                key={m.path}
                onClick={() => selectModel(m)}
                className={`w-full text-left px-4 py-2.5 hover:bg-gray-50 ${
                  selected?.path === m.path ? "bg-blue-50" : ""
                }`}
              >
                <div className="flex items-center gap-2">
                  <Database size={14} className="text-gray-400 shrink-0" />
                  <div>
                    <div className="text-sm font-medium text-gray-800">
                      {m.model.table}
                    </div>
                    <div className="text-xs text-gray-400">
                      {Object.keys(m.model.columns).length} columns
                    </div>
                  </div>
                </div>
              </button>
            ))}
            {models.length === 0 && (
              <div className="p-4 text-sm text-gray-400">
                No models defined.
              </div>
            )}
          </div>

          {/* Generate Migration button */}
          <div className="px-4 py-3 border-t border-gray-200">
            <button
              onClick={() => handleGenerateMigration(false)}
              disabled={models.length === 0}
              className="w-full flex items-center justify-center gap-1 px-3 py-1.5 text-xs text-white bg-indigo-500 rounded hover:bg-indigo-600 disabled:opacity-40"
            >
              Generate Migration
            </button>
          </div>
        </div>

        {/* Editor pane */}
        <div className="flex-1 flex flex-col min-h-0">
          {selected ? (
            <>
              {/* Tab bar */}
              <div className="px-4 py-2 border-b border-gray-200 flex items-center justify-between bg-gray-50 shrink-0">
                <div className="flex items-center gap-2">
                  <div className="flex items-center gap-0.5 bg-gray-100 rounded p-0.5">
                    {(["editor", "diagram", "json"] as EditorTab[]).map(
                      (tab) => (
                        <button
                          key={tab}
                          onClick={() => setActiveTab(tab)}
                          className={`px-2 py-0.5 text-xs rounded capitalize ${
                            activeTab === tab
                              ? "bg-white text-gray-800 shadow-sm font-medium"
                              : "text-gray-500 hover:text-gray-700"
                          }`}
                        >
                          {tab === "editor"
                            ? "Table Editor"
                            : tab === "diagram"
                              ? "ER Diagram"
                              : "JSON"}
                        </button>
                      ),
                    )}
                  </div>
                  <span className="text-sm font-medium text-gray-700">
                    {selected.path}
                  </span>
                  {dirty && (
                    <span className="text-xs text-yellow-600 font-medium">
                      (unsaved)
                    </span>
                  )}
                </div>
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => {
                      setShowCRUD(true);
                      setCrudPreview(null);
                    }}
                    className="flex items-center gap-1 px-2 py-1 text-xs text-indigo-600 border border-indigo-300 rounded hover:bg-indigo-50"
                  >
                    <Wand2 size={12} /> Generate CRUD
                  </button>
                  <button
                    onClick={handleSave}
                    disabled={!dirty || saving}
                    className="flex items-center gap-1 px-2 py-1 text-xs text-white bg-blue-500 rounded hover:bg-blue-600 disabled:opacity-40"
                  >
                    <Save size={12} /> {saving ? "Saving..." : "Save"}
                  </button>
                  <button
                    onClick={handleDelete}
                    className="flex items-center gap-1 px-2 py-1 text-xs text-red-600 border border-red-300 rounded hover:bg-red-50"
                  >
                    <Trash2 size={12} />
                  </button>
                </div>
              </div>

              {/* Tab content */}
              <div className="flex-1 min-h-0 overflow-auto">
                {activeTab === "editor" && parsedModel ? (
                  <ModelFormPanel
                    model={parsedModel}
                    allTables={allTables}
                    onChange={onFormChange}
                  />
                ) : activeTab === "diagram" ? (
                  <ReactFlowProvider>
                    <ERDiagramTab
                      models={models}
                      onSelectModel={(path) => {
                        const m = models.find((m) => m.path === path);
                        if (m) selectModel(m);
                        setActiveTab("editor");
                      }}
                    />
                  </ReactFlowProvider>
                ) : (
                  <Editor
                    height="100%"
                    language="json"
                    value={editorValue}
                    onChange={onJsonChange}
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
              Select a model to edit, or click "New" to create one.
            </div>
          )}
        </div>
      </div>

      {showMigration && (
        <MigrationPreviewDialog
          migrationUp={migrationUp}
          migrationDown={migrationDown}
          onClose={() => setShowMigration(false)}
          onConfirm={() => handleGenerateMigration(true)}
        />
      )}

      {showCRUD && selected && (
        <CRUDGenerationDialog
          tableName={parsedModel?.table ?? ""}
          operations={crudOps}
          artifacts={crudArtifacts}
          service={crudService}
          basePath={crudBasePath}
          scopeCol={crudScopeCol}
          scopeParam={crudScopeParam}
          preview={crudPreview}
          onToggleOp={toggleCrudOp}
          onToggleArtifact={toggleCrudArtifact}
          onServiceChange={setCrudService}
          onBasePathChange={setCrudBasePath}
          onScopeColChange={setCrudScopeCol}
          onScopeParamChange={setCrudScopeParam}
          onPreview={() => handleGenerateCRUD(false)}
          onConfirm={() => handleGenerateCRUD(true)}
          onClose={() => {
            setShowCRUD(false);
            setCrudPreview(null);
          }}
        />
      )}
    </div>
  );
}
