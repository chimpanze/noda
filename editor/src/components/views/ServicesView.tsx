import { useEffect, useState, useCallback } from "react";
import { CheckCircle, XCircle, HelpCircle, RefreshCw, Plus, Trash2 } from "lucide-react";
import { ViewHeader } from "@/components/layout/ViewHeader";
import * as api from "@/api/client";
import { useEditorStore } from "@/stores/editor";
import { showToast } from "@/components/panels/Toast";
import type { ServiceInfo, PluginInfo } from "@/types";

type Tab = "registered" | "defined";

export function ServicesView() {
  const [tab, setTab] = useState<Tab>("registered");

  return (
    <div className="flex-1 flex flex-col min-h-0">
      <ViewHeader title="Services" subtitle="Plugin service instances and health status" />
      {/* Tab bar */}
      <div className="border-b border-gray-200 px-6 flex gap-4">
        {(["registered", "defined"] as const).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`py-2.5 text-sm font-medium border-b-2 transition-colors ${
              tab === t
                ? "border-blue-500 text-blue-600"
                : "border-transparent text-gray-500 hover:text-gray-700"
            }`}
          >
            {t === "registered" ? "Registered" : "Defined"}
          </button>
        ))}
      </div>

      {tab === "registered" ? <RegisteredTab /> : <DefinedTab />}
    </div>
  );
}

/* ─── Registered tab (read-only health view) ─── */

function RegisteredTab() {
  const [services, setServices] = useState<ServiceInfo[]>([]);
  const [plugins, setPlugins] = useState<PluginInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);

  const loadData = useCallback(async () => {
    try {
      const [svc, plg] = await Promise.all([api.listServices(), api.listPlugins()]);
      setServices(svc);
      setPlugins(plg);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    loadData();
    const interval = setInterval(loadData, 30000);
    return () => clearInterval(interval);
  }, [loadData]);

  const refresh = () => {
    setRefreshing(true);
    loadData();
  };

  const grouped = new Map<string, ServiceInfo[]>();
  for (const svc of services) {
    const key = svc.prefix || "other";
    if (!grouped.has(key)) grouped.set(key, []);
    grouped.get(key)!.push(svc);
  }

  if (loading) {
    return <div className="p-6 text-sm text-gray-400">Loading services...</div>;
  }

  return (
    <div className="flex-1 overflow-y-auto p-6">
      <div className="max-w-3xl">
        <div className="flex items-center justify-between mb-6">
          <h2 className="text-lg font-semibold text-gray-900">Registered Services</h2>
          <button
            onClick={refresh}
            disabled={refreshing}
            className="flex items-center gap-1.5 text-xs text-gray-500 hover:text-gray-700 disabled:opacity-50"
          >
            <RefreshCw size={14} className={refreshing ? "animate-spin" : ""} />
            Refresh
          </button>
        </div>

        <div className="mb-6 grid grid-cols-3 gap-3">
          {plugins.map((p) => (
            <div key={p.prefix} className="border border-gray-200 rounded-lg p-3">
              <div className="text-sm font-medium text-gray-800">{p.name}</div>
              <div className="text-xs text-gray-400 mt-0.5">
                {p.prefix}.* · {p.node_count} nodes
              </div>
              {p.description && (
                <div className="text-xs text-gray-400 mt-1">{p.description}</div>
              )}
            </div>
          ))}
        </div>

        {Array.from(grouped.entries()).map(([prefix, svcs]) => (
          <div key={prefix} className="mb-6">
            <h3 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-2">
              {prefix}
            </h3>
            <div className="border border-gray-200 rounded-lg divide-y divide-gray-100">
              {svcs.map((svc) => (
                <div key={svc.name} className="px-4 py-3 flex items-center gap-3">
                  <HealthIcon health={svc.health} />
                  <div className="flex-1 min-w-0">
                    <div className="text-sm font-medium text-gray-800">{svc.name}</div>
                    <div className="text-xs text-gray-400">{svc.prefix}</div>
                  </div>
                  <span
                    className={`text-xs px-2 py-0.5 rounded ${
                      svc.health === "healthy"
                        ? "bg-green-50 text-green-700"
                        : svc.health === "unhealthy"
                          ? "bg-red-50 text-red-700"
                          : "bg-gray-50 text-gray-500"
                    }`}
                  >
                    {svc.health}
                  </span>
                  {svc.error && (
                    <span className="text-xs text-red-500 truncate max-w-48" title={svc.error}>
                      {svc.error}
                    </span>
                  )}
                </div>
              ))}
            </div>
          </div>
        ))}

        {services.length === 0 && (
          <div className="text-sm text-gray-400">No services registered.</div>
        )}
      </div>
    </div>
  );
}

/* ─── Defined tab (CRUD for noda.json services) ─── */

interface DefinedService {
  plugin: string;
  config?: Record<string, unknown>;
}

function DefinedTab() {
  const files = useEditorStore((s) => s.files);

  const [plugins, setPlugins] = useState<PluginInfo[]>([]);
  const [rootConfig, setRootConfig] = useState<Record<string, unknown>>({});
  const [rootPath, setRootPath] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  // Selection
  const [selectedName, setSelectedName] = useState<string | null>(null);
  const [isNew, setIsNew] = useState(false);

  // Editor state
  const [editName, setEditName] = useState("");
  const [editPlugin, setEditPlugin] = useState("");
  const [editConfigRows, setEditConfigRows] = useState<{ key: string; value: string }[]>([]);

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      const [plg, rootData] = await Promise.all([
        api.listPlugins(),
        files?.root
          ? (api.readFile(files.root) as Promise<Record<string, unknown>>)
          : Promise.resolve({}),
      ]);
      setPlugins(plg);
      setRootConfig(rootData);
      if (files?.root) setRootPath(files.root);
    } finally {
      setLoading(false);
    }
  }, [files?.root]);

  useEffect(() => {
    reload();
  }, [reload]);

  const services = (rootConfig.services ?? {}) as Record<string, DefinedService>;
  const serviceNames = Object.keys(services);
  const servicePlugins = plugins.filter((p) => p.has_services);

  const selectService = useCallback(
    (name: string) => {
      setSelectedName(name);
      setIsNew(false);
      setEditName(name);
      const svc = services[name];
      setEditPlugin(svc?.plugin ?? "");
      setEditConfigRows(
        Object.entries(svc?.config ?? {}).map(([k, v]) => ({
          key: k,
          value: String(v),
        }))
      );
    },
    [services]
  );

  const startNew = useCallback(() => {
    setSelectedName(null);
    setIsNew(true);
    setEditName("");
    setEditPlugin("");
    setEditConfigRows([]);
  }, []);

  const saveService = useCallback(async () => {
    if (!editName || !editPlugin || !rootPath) return;
    setSaving(true);
    try {
      const updated = structuredClone(rootConfig);
      const svcs = (updated.services ?? {}) as Record<string, unknown>;

      // If renaming, remove old
      if (!isNew && selectedName && selectedName !== editName) {
        delete svcs[selectedName];
      }

      const config: Record<string, string> = {};
      for (const row of editConfigRows) {
        if (row.key.trim()) config[row.key.trim()] = row.value;
      }

      svcs[editName] = {
        plugin: editPlugin,
        ...(Object.keys(config).length > 0 ? { config } : {}),
      };
      updated.services = svcs;

      await api.writeFile(rootPath, updated);
      showToast({ type: "success", message: `Service "${editName}" saved` });
      setIsNew(false);
      await reload();
      setSelectedName(editName);
    } catch (err) {
      showToast({ type: "error", message: `Failed to save service: ${err}` });
    } finally {
      setSaving(false);
    }
  }, [editName, editPlugin, editConfigRows, rootPath, rootConfig, isNew, selectedName, reload]);

  const deleteService = useCallback(async () => {
    if (!selectedName || !rootPath) return;
    if (!confirm(`Delete service "${selectedName}"?`)) return;
    try {
      const updated = structuredClone(rootConfig);
      const svcs = (updated.services ?? {}) as Record<string, unknown>;
      delete svcs[selectedName];
      updated.services = Object.keys(svcs).length > 0 ? svcs : undefined;

      await api.writeFile(rootPath, updated);
      showToast({ type: "success", message: "Service deleted" });
      setSelectedName(null);
      setIsNew(false);
      setEditName("");
      setEditPlugin("");
      setEditConfigRows([]);
      await reload();
    } catch (err) {
      showToast({ type: "error", message: `Failed to delete: ${err}` });
    }
  }, [selectedName, rootPath, rootConfig, reload]);

  if (loading) {
    return <div className="p-6 text-sm text-gray-400">Loading services...</div>;
  }

  const showEditor = selectedName !== null || isNew;

  return (
    <div className="flex-1 flex min-h-0">
      {/* Left sidebar */}
      <div className="w-72 border-r border-gray-200 overflow-y-auto">
        <div className="px-4 py-3 border-b border-gray-200 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-gray-800">Services</h2>
          <button
            onClick={startNew}
            className="flex items-center gap-1 px-2 py-1 text-xs text-blue-600 hover:bg-blue-50 rounded"
          >
            <Plus size={14} />
            New
          </button>
        </div>
        <div className="divide-y divide-gray-100">
          {serviceNames.map((name) => (
            <button
              key={name}
              onClick={() => selectService(name)}
              className={`w-full text-left px-4 py-2.5 hover:bg-gray-50 ${
                selectedName === name && !isNew ? "bg-blue-50" : ""
              }`}
            >
              <div className="text-sm font-medium text-gray-800">{name}</div>
              <div className="text-xs text-gray-400">plugin: {services[name].plugin}</div>
            </button>
          ))}
          {serviceNames.length === 0 && (
            <div className="p-4 text-xs text-gray-400">No services defined.</div>
          )}
        </div>
      </div>

      {/* Right panel */}
      <div className="flex-1 overflow-y-auto p-6">
        {showEditor ? (
          <div className="max-w-2xl space-y-5">
            <div className="flex items-center justify-between">
              <h3 className="text-lg font-semibold text-gray-900">
                {isNew ? "New Service" : editName}
              </h3>
              <div className="flex items-center gap-2">
                {!isNew && (
                  <button
                    onClick={deleteService}
                    className="px-3 py-1.5 text-sm text-red-600 border border-red-300 rounded hover:bg-red-50"
                  >
                    <Trash2 size={14} className="inline mr-1" />
                    Delete
                  </button>
                )}
                <button
                  onClick={saveService}
                  disabled={saving || !editName || !editPlugin}
                  className="px-4 py-1.5 text-sm text-white bg-blue-500 rounded hover:bg-blue-600 disabled:opacity-50"
                >
                  {saving ? "Saving..." : "Save"}
                </button>
              </div>
            </div>

            <FieldLabel label="Name">
              <input
                type="text"
                value={editName}
                onChange={(e) => setEditName(e.target.value)}
                className="input-field font-mono"
                placeholder="e.g. main-db"
              />
            </FieldLabel>

            <FieldLabel label="Plugin">
              <select
                value={editPlugin}
                onChange={(e) => setEditPlugin(e.target.value)}
                className="input-field"
              >
                <option value="">Select plugin...</option>
                {servicePlugins.map((p) => (
                  <option key={p.prefix} value={p.prefix}>
                    {p.prefix} ({p.name}){p.description ? ` — ${p.description}` : ""}
                  </option>
                ))}
              </select>
            </FieldLabel>

            <FieldLabel label="Config">
              <div className="space-y-2">
                {editConfigRows.map((row, i) => (
                  <div key={i} className="flex items-center gap-2">
                    <input
                      type="text"
                      value={row.key}
                      onChange={(e) => {
                        const rows = [...editConfigRows];
                        rows[i] = { ...rows[i], key: e.target.value };
                        setEditConfigRows(rows);
                      }}
                      className="input-field font-mono flex-1"
                      placeholder="key"
                    />
                    <input
                      type="text"
                      value={row.value}
                      onChange={(e) => {
                        const rows = [...editConfigRows];
                        rows[i] = { ...rows[i], value: e.target.value };
                        setEditConfigRows(rows);
                      }}
                      className="input-field font-mono flex-[2]"
                      placeholder="value"
                    />
                    <button
                      onClick={() => setEditConfigRows(editConfigRows.filter((_, j) => j !== i))}
                      className="text-gray-400 hover:text-red-500 shrink-0"
                    >
                      <Trash2 size={14} />
                    </button>
                  </div>
                ))}
                <button
                  onClick={() => setEditConfigRows([...editConfigRows, { key: "", value: "" }])}
                  className="flex items-center gap-1 text-xs text-blue-600 hover:text-blue-700"
                >
                  <Plus size={12} />
                  Add field
                </button>
              </div>
            </FieldLabel>
          </div>
        ) : (
          <div className="text-sm text-gray-400">
            Select a service to edit, or create a new one.
          </div>
        )}
      </div>
    </div>
  );
}

/* ─── Shared components ─── */

function FieldLabel({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <label className="text-xs font-medium text-gray-400 uppercase block mb-1">
        {label}
      </label>
      {children}
    </div>
  );
}

function HealthIcon({ health }: { health: string }) {
  switch (health) {
    case "healthy":
      return <CheckCircle size={16} className="text-green-500 shrink-0" />;
    case "unhealthy":
      return <XCircle size={16} className="text-red-500 shrink-0" />;
    default:
      return <HelpCircle size={16} className="text-gray-400 shrink-0" />;
  }
}
