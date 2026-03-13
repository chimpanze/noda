import { useEffect, useState, useCallback } from "react";
import { Plus, Trash2, Cpu } from "lucide-react";
import { ViewHeader } from "@/components/layout/ViewHeader";
import Editor from "@monaco-editor/react";
import * as api from "@/api/client";
import { useEditorStore } from "@/stores/editor";
import { showToast } from "@/utils/toast";

interface WasmRuntimeConfig {
  module: string;
  tick_rate: number;
  encoding?: "json" | "msgpack";
  services?: string[];
  connections?: string[];
  allow_outbound?: { http?: string[]; ws?: string[] };
  config?: Record<string, unknown>;
}

export function WasmView() {
  const files = useEditorStore((s) => s.files);

  const [runtimes, setRuntimes] = useState<Record<string, WasmRuntimeConfig>>(
    {},
  );
  const [rootConfig, setRootConfig] = useState<Record<string, unknown>>({});
  const [rootPath, setRootPath] = useState<string>("");
  const [selectedName, setSelectedName] = useState<string | null>(null);
  const [editName, setEditName] = useState("");
  const [editRuntime, setEditRuntime] = useState<WasmRuntimeConfig | null>(
    null,
  );
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [isNew, setIsNew] = useState(false);
  const [configJson, setConfigJson] = useState("");
  const [configError, setConfigError] = useState<string | null>(null);
  const [serviceNames, setServiceNames] = useState<string[]>([]);
  const [connectionNames, setConnectionNames] = useState<string[]>([]);

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      // Read root config (noda.json) to find wasm_runtimes
      const root = files?.root;
      if (!root) {
        setLoading(false);
        return;
      }
      setRootPath(root);
      const [data, services] = await Promise.all([
        api.readFile(root) as Promise<Record<string, unknown>>,
        api.listServices(),
      ]);
      setRootConfig(data);
      const wasm = (data.wasm_runtimes ?? {}) as Record<
        string,
        WasmRuntimeConfig
      >;
      setRuntimes(wasm);
      setServiceNames(services.map((s) => s.name));

      // Load connection endpoint names
      const connNames: string[] = [];
      if (files?.connections) {
        await Promise.all(
          files.connections.map(async (path) => {
            const connData = (await api.readFile(path)) as {
              endpoints?: Record<string, unknown>;
            };
            if (connData.endpoints) {
              connNames.push(...Object.keys(connData.endpoints));
            }
          }),
        );
      }
      setConnectionNames(connNames);
    } catch {
      // root config might not exist or not have wasm_runtimes
      setRuntimes({});
    } finally {
      setLoading(false);
    }
  }, [files?.root, files?.connections]);

  useEffect(() => {
    reload();
  }, [reload]);

  const entries = Object.entries(runtimes);

  const selectRuntime = useCallback(
    (name: string) => {
      setSelectedName(name);
      setEditName(name);
      const rt = structuredClone(runtimes[name]);
      setEditRuntime(rt);
      setConfigJson(JSON.stringify(rt.config ?? {}, null, 2));
      setConfigError(null);
      setIsNew(false);
    },
    [runtimes],
  );

  const startNew = useCallback(() => {
    setSelectedName(null);
    setIsNew(true);
    setEditName("");
    setEditRuntime({
      module: "",
      tick_rate: 10,
      encoding: "json",
      services: [],
      connections: [],
    });
    setConfigJson("{}");
    setConfigError(null);
  }, []);

  const handleSave = useCallback(async () => {
    if (!editRuntime || !editName || !rootPath) return;
    setSaving(true);
    try {
      // Parse custom config
      let customConfig: Record<string, unknown> | undefined;
      try {
        const parsed = JSON.parse(configJson);
        customConfig = Object.keys(parsed).length > 0 ? parsed : undefined;
      } catch {
        showToast({ type: "error", message: "Invalid JSON in custom config" });
        setSaving(false);
        return;
      }

      const clean = structuredClone(editRuntime);
      if (!clean.encoding || clean.encoding === "json") delete clean.encoding;
      if (!clean.services?.length) delete clean.services;
      if (!clean.connections?.length) delete clean.connections;
      if (
        !clean.allow_outbound?.http?.length &&
        !clean.allow_outbound?.ws?.length
      )
        delete clean.allow_outbound;
      else {
        if (!clean.allow_outbound?.http?.length)
          delete clean.allow_outbound!.http;
        if (!clean.allow_outbound?.ws?.length) delete clean.allow_outbound!.ws;
      }
      clean.config = customConfig;
      if (!clean.config) delete clean.config;

      // Update root config
      const updated = structuredClone(rootConfig);
      const wasmMap = (updated.wasm_runtimes ?? {}) as Record<
        string,
        WasmRuntimeConfig
      >;

      // If renaming, remove old
      if (!isNew && selectedName && selectedName !== editName) {
        delete wasmMap[selectedName];
      }
      wasmMap[editName] = clean;
      updated.wasm_runtimes = wasmMap;

      await api.writeFile(rootPath, updated);
      showToast({
        type: "success",
        message: `Wasm runtime "${editName}" saved`,
      });
      setIsNew(false);
      await reload();
      setSelectedName(editName);
    } catch (err) {
      showToast({ type: "error", message: `Failed to save: ${err}` });
    } finally {
      setSaving(false);
    }
  }, [
    editRuntime,
    editName,
    rootPath,
    rootConfig,
    configJson,
    isNew,
    selectedName,
    reload,
  ]);

  const handleDelete = useCallback(async () => {
    if (!selectedName || !rootPath) return;
    if (!confirm(`Delete Wasm runtime "${selectedName}"?`)) return;
    try {
      const updated = structuredClone(rootConfig);
      const wasmMap = (updated.wasm_runtimes ?? {}) as Record<
        string,
        WasmRuntimeConfig
      >;
      delete wasmMap[selectedName];
      updated.wasm_runtimes =
        Object.keys(wasmMap).length > 0 ? wasmMap : undefined;
      await api.writeFile(rootPath, updated);
      showToast({ type: "success", message: `Wasm runtime deleted` });
      setSelectedName(null);
      setEditRuntime(null);
      await reload();
    } catch (err) {
      showToast({ type: "error", message: `Failed to delete: ${err}` });
    }
  }, [selectedName, rootPath, rootConfig, reload]);

  const update = useCallback(
    (patch: Partial<WasmRuntimeConfig>) => {
      if (editRuntime) setEditRuntime({ ...editRuntime, ...patch });
    },
    [editRuntime],
  );

  if (loading) {
    return (
      <div className="p-6 text-sm text-gray-400">Loading Wasm runtimes...</div>
    );
  }

  return (
    <div className="flex-1 flex flex-col min-h-0">
      <ViewHeader
        title="Wasm"
        subtitle="WebAssembly module configuration and management"
      />
      <div className="flex-1 flex min-h-0">
        {/* Runtime list */}
        <div className="w-72 border-r border-gray-200 overflow-y-auto">
          <div className="px-4 py-3 border-b border-gray-200 flex items-center justify-between">
            <h2 className="text-sm font-semibold text-gray-800">
              Wasm Runtimes ({entries.length})
            </h2>
            <button
              onClick={startNew}
              className="flex items-center gap-1 px-2 py-1 text-xs text-blue-600 hover:bg-blue-50 rounded"
            >
              <Plus size={14} />
              New
            </button>
          </div>
          <div className="divide-y divide-gray-100">
            {entries.map(([name, rt]) => (
              <button
                key={name}
                onClick={() => selectRuntime(name)}
                className={`w-full text-left px-4 py-2.5 hover:bg-gray-50 ${
                  selectedName === name && !isNew ? "bg-blue-50" : ""
                }`}
              >
                <div className="flex items-center gap-2">
                  <Cpu size={12} className="text-emerald-500 shrink-0" />
                  <div className="min-w-0">
                    <div className="text-sm font-medium text-gray-800 truncate">
                      {name}
                    </div>
                    <div className="text-xs text-gray-400 truncate">
                      {rt.tick_rate}Hz &middot; {rt.encoding ?? "json"}
                    </div>
                  </div>
                </div>
              </button>
            ))}
            {entries.length === 0 && (
              <div className="p-4 text-sm text-gray-400">
                No Wasm runtimes configured.
              </div>
            )}
          </div>
        </div>

        {/* Runtime editor */}
        <div className="flex-1 overflow-y-auto p-6">
          {editRuntime ? (
            <div className="max-w-2xl space-y-5">
              <div className="flex items-center justify-between">
                <h3 className="text-lg font-semibold text-gray-900">
                  {isNew ? "New Wasm Runtime" : editName}
                </h3>
                <div className="flex items-center gap-2">
                  {!isNew && (
                    <button
                      onClick={handleDelete}
                      className="px-3 py-1.5 text-sm text-red-600 border border-red-300 rounded hover:bg-red-50"
                    >
                      <Trash2 size={14} className="inline mr-1" />
                      Delete
                    </button>
                  )}
                  <button
                    onClick={handleSave}
                    disabled={saving || !editName || !editRuntime.module}
                    className="px-4 py-1.5 text-sm text-white bg-blue-500 rounded hover:bg-blue-600 disabled:opacity-50"
                  >
                    {saving ? "Saving..." : "Save"}
                  </button>
                </div>
              </div>

              {/* Name */}
              <Field label="Name">
                <input
                  type="text"
                  value={editName}
                  onChange={(e) => setEditName(e.target.value)}
                  className="input-field font-mono"
                  placeholder="e.g. game-server"
                />
              </Field>

              {/* Module */}
              <Field label="Module Path">
                <input
                  type="text"
                  value={editRuntime.module}
                  onChange={(e) => update({ module: e.target.value })}
                  className="input-field font-mono"
                  placeholder="wasm/my_module.wasm"
                />
              </Field>

              {/* Tick Rate + Encoding */}
              <div className="grid grid-cols-2 gap-3">
                <Field label="Tick Rate (Hz)">
                  <input
                    type="number"
                    min={1}
                    max={120}
                    value={editRuntime.tick_rate}
                    onChange={(e) =>
                      update({ tick_rate: parseInt(e.target.value, 10) || 10 })
                    }
                    className="input-field"
                  />
                </Field>
                <Field label="Encoding">
                  <select
                    value={editRuntime.encoding ?? "json"}
                    onChange={(e) =>
                      update({ encoding: e.target.value as "json" | "msgpack" })
                    }
                    className="input-field"
                  >
                    <option value="json">JSON</option>
                    <option value="msgpack">MessagePack</option>
                  </select>
                </Field>
              </div>

              {/* Services */}
              <Field label="Service Access">
                <DropdownPicker
                  selected={editRuntime.services ?? []}
                  options={serviceNames}
                  onChange={(services) => update({ services })}
                  placeholder="Add service..."
                />
              </Field>

              {/* Connections */}
              <Field label="Connection Access">
                <DropdownPicker
                  selected={editRuntime.connections ?? []}
                  options={connectionNames}
                  onChange={(connections) => update({ connections })}
                  placeholder="Add connection..."
                />
              </Field>

              {/* Outbound */}
              <div className="border-t border-gray-200 pt-4">
                <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-3">
                  Outbound Whitelist
                </h4>
                <Field label="HTTP Hosts">
                  <StringListEditor
                    values={editRuntime.allow_outbound?.http ?? []}
                    onChange={(http) =>
                      update({
                        allow_outbound: { ...editRuntime.allow_outbound, http },
                      })
                    }
                    placeholder="e.g. api.example.com"
                  />
                </Field>
                <div className="mt-3">
                  <Field label="WebSocket Hosts">
                    <StringListEditor
                      values={editRuntime.allow_outbound?.ws ?? []}
                      onChange={(ws) =>
                        update({
                          allow_outbound: { ...editRuntime.allow_outbound, ws },
                        })
                      }
                      placeholder="e.g. gateway.discord.gg"
                    />
                  </Field>
                </div>
              </div>

              {/* Custom Config (JSON) */}
              <div className="border-t border-gray-200 pt-4">
                <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-2">
                  Custom Config
                  {configError && (
                    <span className="ml-2 text-red-500 normal-case font-normal">
                      Invalid JSON
                    </span>
                  )}
                </h4>
                <div className="border border-gray-300 rounded overflow-hidden">
                  <Editor
                    height="120px"
                    language="json"
                    value={configJson}
                    onChange={(v) => {
                      const val = v ?? "";
                      setConfigJson(val);
                      try {
                        JSON.parse(val);
                        setConfigError(null);
                      } catch (e) {
                        setConfigError((e as Error).message);
                      }
                    }}
                    options={{
                      minimap: { enabled: false },
                      fontSize: 13,
                      scrollBeyondLastLine: false,
                      wordWrap: "on",
                      lineNumbers: "on",
                    }}
                  />
                </div>
              </div>
            </div>
          ) : (
            <div className="text-sm text-gray-400">
              Select a Wasm runtime to edit or click "New" to create one.
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function Field({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div>
      <label className="text-xs font-medium text-gray-400 uppercase block mb-1">
        {label}
      </label>
      {children}
    </div>
  );
}

function DropdownPicker({
  selected,
  options,
  onChange,
  placeholder,
}: {
  selected: string[];
  options: string[];
  onChange: (values: string[]) => void;
  placeholder?: string;
}) {
  return (
    <>
      <div className="flex flex-wrap gap-1.5 mb-1.5">
        {selected.map((v) => (
          <span
            key={v}
            className="inline-flex items-center gap-1 px-2 py-0.5 text-xs bg-gray-100 text-gray-700 rounded font-mono"
          >
            {v}
            <button
              type="button"
              onClick={() => onChange(selected.filter((x) => x !== v))}
              className="text-gray-400 hover:text-gray-600"
            >
              &times;
            </button>
          </span>
        ))}
      </div>
      <select
        value=""
        onChange={(e) => {
          if (!e.target.value) return;
          onChange([...selected, e.target.value]);
        }}
        className="input-field"
      >
        <option value="">{placeholder ?? "Add..."}</option>
        {options
          .filter((n) => !selected.includes(n))
          .map((n) => (
            <option key={n} value={n}>
              {n}
            </option>
          ))}
      </select>
    </>
  );
}

function StringListEditor({
  values,
  onChange,
  placeholder,
}: {
  values: string[];
  onChange: (values: string[]) => void;
  placeholder?: string;
}) {
  const [draft, setDraft] = useState("");

  const add = () => {
    const v = draft.trim();
    if (v && !values.includes(v)) onChange([...values, v]);
    setDraft("");
  };

  return (
    <>
      <div className="flex flex-wrap gap-1.5 mb-1.5">
        {values.map((v) => (
          <span
            key={v}
            className="inline-flex items-center gap-1 px-2 py-0.5 text-xs bg-gray-100 text-gray-700 rounded font-mono"
          >
            {v}
            <button
              type="button"
              onClick={() => onChange(values.filter((x) => x !== v))}
              className="text-gray-400 hover:text-gray-600"
            >
              &times;
            </button>
          </span>
        ))}
      </div>
      <div className="flex gap-1">
        <input
          type="text"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              add();
            }
          }}
          className="input-field flex-1 font-mono"
          placeholder={placeholder}
        />
      </div>
    </>
  );
}
