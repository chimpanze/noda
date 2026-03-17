import { useEffect, useState, useCallback } from "react";
import { useConfigSection } from "@/hooks/useConfigSection";
import {
  CheckCircle,
  XCircle,
  HelpCircle,
  RefreshCw,
  Plus,
  Trash2,
} from "lucide-react";
import { ViewHeader } from "@/components/layout/ViewHeader";
import { Field } from "@/components/ui/Field";
import { DetailHeader } from "@/components/ui/DetailHeader";
import { ConfigListDetail } from "@/components/ui/ConfigListDetail";
import * as api from "@/api/client";
import { showToast } from "@/utils/toast";
import type { ServiceInfo, PluginInfo } from "@/types";

type Tab = "registered" | "defined";

export function ServicesView() {
  const [tab, setTab] = useState<Tab>("registered");

  return (
    <div className="flex-1 flex flex-col min-h-0">
      <ViewHeader
        title="Services"
        subtitle="Plugin service instances and health status"
      />
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
      const [svc, plg] = await Promise.all([
        api.listServices(),
        api.listPlugins(),
      ]);
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
          <h2 className="text-lg font-semibold text-gray-900">
            Registered Services
          </h2>
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
            <div
              key={p.prefix}
              className="border border-gray-200 rounded-lg p-3"
            >
              <div className="text-sm font-medium text-gray-800">{p.name}</div>
              <div className="text-xs text-gray-400 mt-0.5">
                {p.prefix}.* · {p.node_count} nodes
              </div>
              {p.description && (
                <div className="text-xs text-gray-400 mt-1">
                  {p.description}
                </div>
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
                <div
                  key={svc.name}
                  className="px-4 py-3 flex items-center gap-3"
                >
                  <HealthIcon health={svc.health} />
                  <div className="flex-1 min-w-0">
                    <div className="text-sm font-medium text-gray-800">
                      {svc.name}
                    </div>
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
                    <span
                      className="text-xs text-red-500 truncate max-w-48"
                      title={svc.error}
                    >
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
  const {
    data: services,
    loading,
    set: setService,
    remove: removeService,
    replace: replaceServices,
    reload,
  } = useConfigSection<Record<string, DefinedService>>({ path: "services" });

  const [plugins, setPlugins] = useState<PluginInfo[]>([]);
  const [saving, setSaving] = useState(false);

  // Selection
  const [selectedName, setSelectedName] = useState<string | null>(null);
  const [isNew, setIsNew] = useState(false);

  // Editor state
  const [editName, setEditName] = useState("");
  const [editPlugin, setEditPlugin] = useState("");
  const [editConfigRows, setEditConfigRows] = useState<
    { key: string; value: string }[]
  >([]);

  useEffect(() => {
    api.listPlugins().then(setPlugins);
  }, []);
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
        })),
      );
    },
    [services],
  );

  const startNew = useCallback(() => {
    setSelectedName(null);
    setIsNew(true);
    setEditName("");
    setEditPlugin("");
    setEditConfigRows([]);
  }, []);

  const saveService = useCallback(async () => {
    if (!editName || !editPlugin) return;
    setSaving(true);
    try {
      const config: Record<string, string> = {};
      for (const row of editConfigRows) {
        if (row.key.trim()) config[row.key.trim()] = row.value;
      }

      const svcValue = {
        plugin: editPlugin,
        ...(Object.keys(config).length > 0 ? { config } : {}),
      };

      if (!isNew && selectedName && selectedName !== editName) {
        // Rename: atomic remove old + add new
        const updated = { ...services };
        delete updated[selectedName];
        updated[editName] = svcValue;
        await replaceServices(updated);
      } else {
        await setService(editName, svcValue);
      }

      showToast({ type: "success", message: `Service "${editName}" saved` });
      setIsNew(false);
      await reload();
      setSelectedName(editName);
    } catch (err) {
      showToast({ type: "error", message: `Failed to save service: ${err}` });
    } finally {
      setSaving(false);
    }
  }, [
    editName,
    editPlugin,
    editConfigRows,
    isNew,
    selectedName,
    services,
    replaceServices,
    setService,
    reload,
  ]);

  const deleteService = useCallback(async () => {
    if (!selectedName) return;
    if (!confirm(`Delete service "${selectedName}"?`)) return;
    try {
      await removeService(selectedName);
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
  }, [selectedName, removeService, reload]);

  if (loading) {
    return <div className="p-6 text-sm text-gray-400">Loading services...</div>;
  }

  const showEditor = selectedName !== null || isNew;

  return (
    <ConfigListDetail
      items={serviceNames}
      getKey={(name) => name}
      selectedKey={isNew ? null : selectedName}
      onSelect={(key) => selectService(key)}
      renderItem={(name) => (
        <>
          <div className="text-sm font-medium text-gray-800">{name}</div>
          <div className="text-xs text-gray-400">
            plugin: {services[name].plugin}
          </div>
        </>
      )}
      title="Services"
      onNew={startNew}
      sidebarWidth="w-72"
      emptyMessage="No services defined."
    >
      {showEditor ? (
          <div className="max-w-2xl space-y-5">
            <DetailHeader
              title={isNew ? "New Service" : editName}
              isNew={isNew}
              saving={saving}
              onSave={saveService}
              onDelete={deleteService}
              saveDisabled={!editName || !editPlugin}
            />

            <Field label="Name">
              <input
                type="text"
                value={editName}
                onChange={(e) => setEditName(e.target.value)}
                className="input-field font-mono"
                placeholder="e.g. main-db"
              />
            </Field>

            <Field label="Plugin">
              <select
                value={editPlugin}
                onChange={(e) => setEditPlugin(e.target.value)}
                className="input-field"
              >
                <option value="">Select plugin...</option>
                {servicePlugins.map((p) => (
                  <option key={p.name} value={p.name}>
                    {p.name} ({p.prefix}.*)
                    {p.description ? ` — ${p.description}` : ""}
                  </option>
                ))}
              </select>
            </Field>

            <Field label="Config">
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
                      onClick={() =>
                        setEditConfigRows(
                          editConfigRows.filter((_, j) => j !== i),
                        )
                      }
                      className="text-gray-400 hover:text-red-500 shrink-0"
                    >
                      <Trash2 size={14} />
                    </button>
                  </div>
                ))}
                <button
                  onClick={() =>
                    setEditConfigRows([
                      ...editConfigRows,
                      { key: "", value: "" },
                    ])
                  }
                  className="flex items-center gap-1 text-xs text-blue-600 hover:text-blue-700"
                >
                  <Plus size={12} />
                  Add field
                </button>
              </div>
            </Field>
          </div>
        ) : (
          <div className="text-sm text-gray-400">
            Select a service to edit, or create a new one.
          </div>
        )}
    </ConfigListDetail>
  );
}

/* ─── Shared components ─── */

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
