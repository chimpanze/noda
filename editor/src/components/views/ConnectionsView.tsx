import { useEffect, useState, useCallback } from "react";
import { Plus, Trash2, ExternalLink, Wifi, Radio } from "lucide-react";
import * as api from "@/api/client";
import { useEditorStore } from "@/stores/editor";
import { showToast } from "@/components/panels/Toast";

interface EndpointConfig {
  type: "websocket" | "sse";
  path: string;
  middleware?: string[];
  channels?: {
    pattern?: string;
    max_per_channel?: number;
  };
  ping_interval?: string;
  on_connect?: string;
  on_message?: string;
  on_disconnect?: string;
  [key: string]: unknown;
}

interface ConnectionFileConfig {
  sync?: { pubsub?: string };
  endpoints?: Record<string, EndpointConfig>;
  [key: string]: unknown;
}

interface EndpointEntry {
  filePath: string;
  name: string;
  endpoint: EndpointConfig;
}

export function ConnectionsView() {
  const files = useEditorStore((s) => s.files);
  const loadFiles = useEditorStore((s) => s.loadFiles);
  const setActiveView = useEditorStore((s) => s.setActiveView);
  const setActiveWorkflow = useEditorStore((s) => s.setActiveWorkflow);

  const [connFiles, setConnFiles] = useState<{ path: string; config: ConnectionFileConfig }[]>([]);
  const [entries, setEntries] = useState<EndpointEntry[]>([]);
  const [selectedIndex, setSelectedIndex] = useState<number | null>(null);
  const [editName, setEditName] = useState("");
  const [editEndpoint, setEditEndpoint] = useState<EndpointConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [isNew, setIsNew] = useState(false);
  const [middlewareNames, setMiddlewareNames] = useState<string[]>([]);
  const [instanceNames, setInstanceNames] = useState<string[]>([]);

  const reload = useCallback(async () => {
    if (!files?.connections) return;
    setLoading(true);
    try {
      const fileConfigs: { path: string; config: ConnectionFileConfig }[] = [];
      const allEntries: EndpointEntry[] = [];
      await Promise.all(
        files.connections.map(async (path) => {
          const data = (await api.readFile(path)) as ConnectionFileConfig;
          fileConfigs.push({ path, config: data });
          if (data.endpoints) {
            for (const [name, endpoint] of Object.entries(data.endpoints)) {
              allEntries.push({ filePath: path, name, endpoint });
            }
          }
        })
      );
      setConnFiles(fileConfigs);
      setEntries(allEntries);

      const mwInfo = await api.listMiddleware();
      const instNames = Object.keys(mwInfo.instances ?? {});
      setInstanceNames(instNames);
      setMiddlewareNames([...mwInfo.middleware.map((m) => m.name), ...instNames]);
    } finally {
      setLoading(false);
    }
  }, [files?.connections]);

  useEffect(() => {
    reload();
  }, [reload]);

  const selectEndpoint = useCallback(
    (index: number) => {
      setSelectedIndex(index);
      setEditName(entries[index].name);
      setEditEndpoint(structuredClone(entries[index].endpoint));
      setIsNew(false);
    },
    [entries]
  );

  const startNew = useCallback(() => {
    setSelectedIndex(null);
    setIsNew(true);
    setEditName("");
    setEditEndpoint({
      type: "websocket",
      path: "/ws/",
    });
  }, []);

  const handleSave = useCallback(async () => {
    if (!editEndpoint || !editName) return;
    setSaving(true);
    try {
      const clean = structuredClone(editEndpoint);
      if (!clean.middleware?.length) delete clean.middleware;
      if (clean.channels && !clean.channels.pattern) delete clean.channels;
      if (!clean.ping_interval) delete clean.ping_interval;
      if (!clean.on_connect) delete clean.on_connect;
      if (!clean.on_message) delete clean.on_message;
      if (!clean.on_disconnect) delete clean.on_disconnect;

      if (isNew) {
        // Add to first connection file, or create one
        let targetFile = connFiles[0];
        if (!targetFile) {
          const newConfig: ConnectionFileConfig = { endpoints: {} };
          const path = "connections/default.json";
          targetFile = { path, config: newConfig };
        }
        const updated = structuredClone(targetFile.config);
        if (!updated.endpoints) updated.endpoints = {};
        updated.endpoints[editName] = clean;
        await api.writeFile(targetFile.path, updated);
        showToast({ type: "success", message: `Endpoint "${editName}" created` });
      } else if (selectedIndex !== null) {
        const entry = entries[selectedIndex];
        const fileConfig = connFiles.find((f) => f.path === entry.filePath);
        if (fileConfig) {
          const updated = structuredClone(fileConfig.config);
          if (updated.endpoints) {
            // If name changed, remove old key
            if (entry.name !== editName) {
              delete updated.endpoints[entry.name];
            }
            updated.endpoints[editName] = clean;
          }
          await api.writeFile(fileConfig.path, updated);
          showToast({ type: "success", message: `Endpoint "${editName}" saved` });
        }
      }

      setIsNew(false);
      await loadFiles();
      await reload();
    } catch (err) {
      showToast({ type: "error", message: `Failed to save: ${err}` });
    } finally {
      setSaving(false);
    }
  }, [editEndpoint, editName, isNew, selectedIndex, entries, connFiles, loadFiles, reload]);

  const handleDelete = useCallback(async () => {
    if (selectedIndex === null) return;
    const entry = entries[selectedIndex];
    if (!confirm(`Delete endpoint "${entry.name}"?`)) return;
    try {
      const fileConfig = connFiles.find((f) => f.path === entry.filePath);
      if (fileConfig) {
        const updated = structuredClone(fileConfig.config);
        if (updated.endpoints) {
          delete updated.endpoints[entry.name];
        }
        await api.writeFile(fileConfig.path, updated);
      }
      showToast({ type: "success", message: `Endpoint deleted` });
      setSelectedIndex(null);
      setEditEndpoint(null);
      await loadFiles();
      await reload();
    } catch (err) {
      showToast({ type: "error", message: `Failed to delete: ${err}` });
    }
  }, [selectedIndex, entries, connFiles, loadFiles, reload]);

  const goToWorkflow = useCallback(
    (workflowId: string) => {
      const wfFiles = files?.workflows ?? [];
      const match = wfFiles.find((f) => f.includes(workflowId));
      if (match) {
        setActiveView("workflows");
        setActiveWorkflow(match);
      }
    },
    [files?.workflows, setActiveView, setActiveWorkflow]
  );

  const update = useCallback(
    (patch: Partial<EndpointConfig>) => {
      if (editEndpoint) setEditEndpoint({ ...editEndpoint, ...patch });
    },
    [editEndpoint]
  );

  if (loading) {
    return <div className="p-6 text-sm text-gray-400">Loading connections...</div>;
  }

  return (
    <div className="flex-1 flex min-h-0">
      {/* Endpoint list */}
      <div className="w-80 border-r border-gray-200 overflow-y-auto">
        <div className="px-4 py-3 border-b border-gray-200 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-gray-800">
            Connections ({entries.length})
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
          {entries.map((entry, index) => (
            <button
              key={`${entry.filePath}-${entry.name}`}
              onClick={() => selectEndpoint(index)}
              className={`w-full text-left px-4 py-2.5 hover:bg-gray-50 ${
                selectedIndex === index && !isNew ? "bg-blue-50" : ""
              }`}
            >
              <div className="flex items-center gap-2">
                {entry.endpoint.type === "websocket" ? (
                  <Wifi size={12} className="text-violet-500 shrink-0" />
                ) : (
                  <Radio size={12} className="text-teal-500 shrink-0" />
                )}
                <div className="min-w-0">
                  <div className="text-sm font-medium text-gray-800 truncate">
                    {entry.name}
                  </div>
                  <div className="text-xs text-gray-400 truncate font-mono">
                    {entry.endpoint.path}
                  </div>
                </div>
              </div>
            </button>
          ))}
          {entries.length === 0 && (
            <div className="p-4 text-sm text-gray-400">No connections configured.</div>
          )}
        </div>
      </div>

      {/* Endpoint editor */}
      <div className="flex-1 overflow-y-auto p-6">
        {editEndpoint ? (
          <div className="max-w-2xl space-y-5">
            <div className="flex items-center justify-between">
              <h3 className="text-lg font-semibold text-gray-900">
                {isNew ? "New Endpoint" : editName}
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
                  disabled={saving || !editName}
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
                placeholder="e.g. tasks-live"
              />
            </Field>

            {/* Type + Path */}
            <div className="grid grid-cols-[160px_1fr] gap-3">
              <Field label="Type">
                <select
                  value={editEndpoint.type}
                  onChange={(e) => update({ type: e.target.value as "websocket" | "sse" })}
                  className="input-field"
                >
                  <option value="websocket">WebSocket</option>
                  <option value="sse">SSE</option>
                </select>
              </Field>
              <Field label="Path">
                <input
                  type="text"
                  value={editEndpoint.path}
                  onChange={(e) => update({ path: e.target.value })}
                  className="input-field font-mono"
                  placeholder="/ws/events"
                />
              </Field>
            </div>

            {/* Middleware */}
            <Field label="Middleware">
              <div className="flex flex-wrap gap-1.5 mb-1.5">
                {(editEndpoint.middleware ?? []).map((mw) => (
                  <span
                    key={mw}
                    className="inline-flex items-center gap-1 px-2 py-0.5 text-xs bg-gray-100 text-gray-700 rounded"
                  >
                    {mw}
                    <button
                      type="button"
                      onClick={() => {
                        const filtered = (editEndpoint.middleware ?? []).filter((x) => x !== mw);
                        update({ middleware: filtered.length > 0 ? filtered : undefined });
                      }}
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
                  const current = editEndpoint.middleware ?? [];
                  update({ middleware: [...current, e.target.value] });
                }}
                className="input-field"
              >
                <option value="">Add middleware...</option>
                {middlewareNames
                  .filter((n) => !instanceNames.includes(n))
                  .filter((n) => !(editEndpoint.middleware ?? []).includes(n))
                  .map((n) => (
                    <option key={n} value={n}>{n}</option>
                  ))}
                {instanceNames.filter((n) => !(editEndpoint.middleware ?? []).includes(n)).length > 0 && (
                  <optgroup label="Instances">
                    {instanceNames
                      .filter((n) => !(editEndpoint.middleware ?? []).includes(n))
                      .map((n) => (
                        <option key={n} value={n}>{n}</option>
                      ))}
                  </optgroup>
                )}
              </select>
            </Field>

            {/* Channels */}
            <div className="border-t border-gray-200 pt-4">
              <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-3">
                Channels
              </h4>
              <div className="grid grid-cols-2 gap-3">
                <Field label="Pattern">
                  <input
                    type="text"
                    value={editEndpoint.channels?.pattern ?? ""}
                    onChange={(e) =>
                      update({
                        channels: {
                          ...editEndpoint.channels,
                          pattern: e.target.value || undefined,
                        },
                      })
                    }
                    className="input-field font-mono"
                    placeholder="{{ auth.sub }}"
                  />
                </Field>
                <Field label="Max Per Channel">
                  <input
                    type="number"
                    min={0}
                    value={editEndpoint.channels?.max_per_channel ?? ""}
                    onChange={(e) => {
                      const val = parseInt(e.target.value, 10);
                      update({
                        channels: {
                          ...editEndpoint.channels,
                          max_per_channel: isNaN(val) ? undefined : val,
                        },
                      });
                    }}
                    className="input-field"
                    placeholder="unlimited"
                  />
                </Field>
              </div>
            </div>

            {/* WebSocket-specific */}
            {editEndpoint.type === "websocket" && (
              <Field label="Ping Interval">
                <input
                  type="text"
                  value={editEndpoint.ping_interval ?? ""}
                  onChange={(e) => update({ ping_interval: e.target.value || undefined })}
                  className="input-field w-48"
                  placeholder="e.g. 30s"
                />
              </Field>
            )}

            {/* Lifecycle Workflows */}
            <div className="border-t border-gray-200 pt-4">
              <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-3">
                Lifecycle Workflows
              </h4>
              <div className="space-y-3">
                {(["on_connect", "on_message", "on_disconnect"] as const).map((hook) => (
                  <Field key={hook} label={hook.replace("_", " ")}>
                    <div className="flex items-center gap-2">
                      <select
                        value={editEndpoint[hook] as string ?? ""}
                        onChange={(e) => update({ [hook]: e.target.value || undefined })}
                        className="input-field flex-1"
                      >
                        <option value="">None</option>
                        {(files?.workflows ?? []).map((wf) => {
                          const name = wf.replace(/^workflows\//, "").replace(/\.json$/, "");
                          return (
                            <option key={wf} value={name}>{name}</option>
                          );
                        })}
                      </select>
                      {editEndpoint[hook] && (
                        <button
                          onClick={() => goToWorkflow(editEndpoint[hook] as string)}
                          className="text-blue-500 hover:text-blue-700"
                          title="Open workflow"
                        >
                          <ExternalLink size={14} />
                        </button>
                      )}
                    </div>
                  </Field>
                ))}
              </div>
            </div>

            {/* JSON Preview */}
            <div className="border-t border-gray-200 pt-4">
              <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-2">
                JSON Preview
              </h4>
              <pre className="p-3 bg-gray-50 rounded text-xs text-gray-600 overflow-x-auto whitespace-pre-wrap border border-gray-200">
                {JSON.stringify({ [editName || "name"]: editEndpoint }, null, 2)}
              </pre>
            </div>
          </div>
        ) : (
          <div className="text-sm text-gray-400">
            Select an endpoint to edit or click "New" to create one.
          </div>
        )}
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

