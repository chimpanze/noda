import { useEffect, useState, useCallback } from "react";
import { Plus, Trash2, X, Eye, EyeOff } from "lucide-react";
import { ViewHeader } from "@/components/layout/ViewHeader";
import * as api from "@/api/client";
import { useEditorStore } from "@/stores/editor";
import { showToast } from "@/components/panels/Toast";
import type { MiddlewareDescriptor, MiddlewareInstance, ConfigField } from "@/types";

type Section = "presets" | "config" | "instances";

export function MiddlewareView() {
  const files = useEditorStore((s) => s.files);

  const [descriptors, setDescriptors] = useState<MiddlewareDescriptor[]>([]);
  const [presets, setPresets] = useState<Record<string, string[]>>({});
  const [mwConfig, setMwConfig] = useState<Record<string, Record<string, unknown>>>({});
  const [instances, setInstances] = useState<Record<string, MiddlewareInstance>>({});
  const [rootConfig, setRootConfig] = useState<Record<string, unknown>>({});
  const [rootPath, setRootPath] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  // Selection state
  const [section, setSection] = useState<Section>("presets");
  const [selectedPreset, setSelectedPreset] = useState<string | null>(null);
  const [selectedMw, setSelectedMw] = useState<string | null>(null);
  const [selectedInstance, setSelectedInstance] = useState<string | null>(null);
  const [isNewPreset, setIsNewPreset] = useState(false);
  const [isNewInstance, setIsNewInstance] = useState(false);

  // Preset editor state
  const [editPresetName, setEditPresetName] = useState("");
  const [editPresetMws, setEditPresetMws] = useState<string[]>([]);

  // Config editor state
  const [editConfig, setEditConfig] = useState<Record<string, unknown>>({});

  // Instance editor state
  const [editInstanceName, setEditInstanceName] = useState("");
  const [editInstanceType, setEditInstanceType] = useState("");
  const [editInstanceConfig, setEditInstanceConfig] = useState<Record<string, unknown>>({});

  // Resolved toggle
  const [showResolved, setShowResolved] = useState(false);

  const configurableMiddleware = descriptors.filter((d) => d.config_fields.length > 0);

  // Extract raw (unresolved) middleware config from rootConfig file,
  // mirroring the Go-side extractMiddlewareConfig lookup paths.
  const extractRawMwConfig = useCallback(
    (name: string): Record<string, unknown> => {
      const sec = rootConfig.security as Record<string, unknown> | undefined;
      const mw = rootConfig.middleware as Record<string, unknown> | undefined;
      if (name.startsWith("security.")) {
        const shortName = name.replace("security.", "");
        return { ...((sec?.[shortName] as Record<string, unknown>) ?? {}) };
      }
      if (name === "auth.jwt") {
        return { ...((sec?.jwt as Record<string, unknown>) ?? {}) };
      }
      if (name === "casbin.enforce") {
        return { ...((sec?.casbin as Record<string, unknown>) ?? {}) };
      }
      return { ...((mw?.[name] as Record<string, unknown>) ?? {}) };
    },
    [rootConfig]
  );

  // Extract raw (unresolved) instance config from rootConfig file.
  const extractRawInstanceConfig = useCallback(
    (key: string): { type: string; config: Record<string, unknown> } | null => {
      const mi = rootConfig.middleware_instances as Record<string, unknown> | undefined;
      if (!mi) return null;
      const entry = mi[key] as { type?: string; config?: Record<string, unknown> } | undefined;
      if (!entry) return null;
      return { type: entry.type ?? "", config: { ...(entry.config ?? {}) } };
    },
    [rootConfig]
  );

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      const [mwInfo, rootData] = await Promise.all([
        api.listMiddleware(),
        files?.root ? api.readFile(files.root) as Promise<Record<string, unknown>> : Promise.resolve({}),
      ]);
      setDescriptors(mwInfo.middleware);
      setPresets(mwInfo.presets);
      setMwConfig(mwInfo.config);
      setInstances(mwInfo.instances ?? {});
      setRootConfig(rootData);
      if (files?.root) setRootPath(files.root);
    } finally {
      setLoading(false);
    }
  }, [files?.root]);

  useEffect(() => {
    reload();
  }, [reload]);

  const selectPreset = useCallback(
    (name: string) => {
      setSection("presets");
      setSelectedPreset(name);
      setSelectedMw(null);
      setIsNewPreset(false);
      setEditPresetName(name);
      setEditPresetMws([...(presets[name] ?? [])]);
    },
    [presets]
  );

  const startNewPreset = useCallback(() => {
    setSection("presets");
    setSelectedPreset(null);
    setSelectedMw(null);
    setIsNewPreset(true);
    setEditPresetName("");
    setEditPresetMws([]);
  }, []);

  const selectConfig = useCallback(
    (name: string) => {
      setSection("config");
      setSelectedMw(name);
      setSelectedPreset(null);
      setSelectedInstance(null);
      setIsNewPreset(false);
      setIsNewInstance(false);
      setShowResolved(false);
      setEditConfig(extractRawMwConfig(name));
    },
    [extractRawMwConfig]
  );

  const selectInstance = useCallback(
    (key: string) => {
      setSection("instances");
      setSelectedInstance(key);
      setSelectedPreset(null);
      setSelectedMw(null);
      setIsNewPreset(false);
      setIsNewInstance(false);
      setShowResolved(false);
      const inst = extractRawInstanceConfig(key);
      if (inst) {
        // Extract name part after ":"
        const colonIdx = key.indexOf(":");
        setEditInstanceName(colonIdx >= 0 ? key.slice(colonIdx + 1) : key);
        setEditInstanceType(inst.type);
        setEditInstanceConfig({ ...inst.config });
      }
    },
    [extractRawInstanceConfig]
  );

  const startNewInstance = useCallback(() => {
    setSection("instances");
    setSelectedInstance(null);
    setSelectedPreset(null);
    setSelectedMw(null);
    setIsNewPreset(false);
    setIsNewInstance(true);
    setEditInstanceName("");
    setEditInstanceType("");
    setEditInstanceConfig({});
  }, []);

  const savePreset = useCallback(async () => {
    if (!editPresetName || !rootPath) return;
    setSaving(true);
    try {
      const updated = structuredClone(rootConfig);
      const mp = (updated.middleware_presets ?? {}) as Record<string, unknown>;

      // If renaming, remove old
      if (!isNewPreset && selectedPreset && selectedPreset !== editPresetName) {
        delete mp[selectedPreset];
      }
      mp[editPresetName] = editPresetMws;
      updated.middleware_presets = mp;

      await api.writeFile(rootPath, updated);
      showToast({ type: "success", message: `Preset "${editPresetName}" saved` });
      setIsNewPreset(false);
      await reload();
      setSelectedPreset(editPresetName);
    } catch (err) {
      showToast({ type: "error", message: `Failed to save preset: ${err}` });
    } finally {
      setSaving(false);
    }
  }, [editPresetName, editPresetMws, rootPath, rootConfig, isNewPreset, selectedPreset, reload]);

  const deletePreset = useCallback(async () => {
    if (!selectedPreset || !rootPath) return;
    if (!confirm(`Delete preset "${selectedPreset}"?`)) return;
    try {
      const updated = structuredClone(rootConfig);
      const mp = (updated.middleware_presets ?? {}) as Record<string, unknown>;
      delete mp[selectedPreset];
      updated.middleware_presets = Object.keys(mp).length > 0 ? mp : undefined;
      await api.writeFile(rootPath, updated);
      showToast({ type: "success", message: "Preset deleted" });
      setSelectedPreset(null);
      setEditPresetName("");
      setEditPresetMws([]);
      await reload();
    } catch (err) {
      showToast({ type: "error", message: `Failed to delete: ${err}` });
    }
  }, [selectedPreset, rootPath, rootConfig, reload]);

  const saveInstance = useCallback(async () => {
    if (!editInstanceName || !editInstanceType || !rootPath) return;
    setSaving(true);
    try {
      const updated = structuredClone(rootConfig);
      const mi = (updated.middleware_instances ?? {}) as Record<string, unknown>;

      const newKey = `${editInstanceType}:${editInstanceName}`;

      // If renaming, remove old
      if (!isNewInstance && selectedInstance && selectedInstance !== newKey) {
        delete mi[selectedInstance];
      }
      mi[newKey] = { type: editInstanceType, config: editInstanceConfig };
      updated.middleware_instances = mi;

      await api.writeFile(rootPath, updated);
      showToast({ type: "success", message: `Instance "${newKey}" saved` });
      setIsNewInstance(false);
      await reload();
      setSelectedInstance(newKey);
    } catch (err) {
      showToast({ type: "error", message: `Failed to save instance: ${err}` });
    } finally {
      setSaving(false);
    }
  }, [editInstanceName, editInstanceType, editInstanceConfig, rootPath, rootConfig, isNewInstance, selectedInstance, reload]);

  const deleteInstance = useCallback(async () => {
    if (!selectedInstance || !rootPath) return;
    if (!confirm(`Delete instance "${selectedInstance}"?`)) return;
    try {
      const updated = structuredClone(rootConfig);
      const mi = (updated.middleware_instances ?? {}) as Record<string, unknown>;
      delete mi[selectedInstance];
      updated.middleware_instances = Object.keys(mi).length > 0 ? mi : undefined;
      await api.writeFile(rootPath, updated);
      showToast({ type: "success", message: "Instance deleted" });
      setSelectedInstance(null);
      setEditInstanceName("");
      setEditInstanceType("");
      setEditInstanceConfig({});
      await reload();
    } catch (err) {
      showToast({ type: "error", message: `Failed to delete: ${err}` });
    }
  }, [selectedInstance, rootPath, rootConfig, reload]);

  const saveConfig = useCallback(async () => {
    if (!selectedMw || !rootPath) return;
    setSaving(true);
    try {
      const updated = structuredClone(rootConfig);

      // Determine where to save based on middleware name
      const cleanConfig = Object.fromEntries(
        Object.entries(editConfig).filter(([, v]) => v !== "" && v !== undefined)
      );
      const hasValues = Object.keys(cleanConfig).length > 0;

      if (selectedMw.startsWith("security.")) {
        const shortName = selectedMw.replace("security.", "");
        const sec = (updated.security ?? {}) as Record<string, unknown>;
        if (hasValues) {
          sec[shortName] = cleanConfig;
        } else {
          delete sec[shortName];
        }
        updated.security = Object.keys(sec).length > 0 ? sec : undefined;
      } else if (selectedMw === "auth.jwt") {
        const sec = (updated.security ?? {}) as Record<string, unknown>;
        if (hasValues) {
          sec.jwt = cleanConfig;
        } else {
          delete sec.jwt;
        }
        updated.security = Object.keys(sec).length > 0 ? sec : undefined;
      } else if (selectedMw === "casbin.enforce") {
        const sec = (updated.security ?? {}) as Record<string, unknown>;
        if (hasValues) {
          sec.casbin = cleanConfig;
        } else {
          delete sec.casbin;
        }
        updated.security = Object.keys(sec).length > 0 ? sec : undefined;
      } else {
        const mw = (updated.middleware ?? {}) as Record<string, unknown>;
        if (hasValues) {
          mw[selectedMw] = cleanConfig;
        } else {
          delete mw[selectedMw];
        }
        updated.middleware = Object.keys(mw).length > 0 ? mw : undefined;
      }

      await api.writeFile(rootPath, updated);
      showToast({ type: "success", message: `Config for "${selectedMw}" saved` });
      await reload();
    } catch (err) {
      showToast({ type: "error", message: `Failed to save config: ${err}` });
    } finally {
      setSaving(false);
    }
  }, [selectedMw, editConfig, rootPath, rootConfig, reload]);

  if (loading) {
    return <div className="p-6 text-sm text-gray-400">Loading middleware...</div>;
  }

  const showPresetEditor = section === "presets" && (selectedPreset !== null || isNewPreset);
  const showConfigEditor = section === "config" && selectedMw !== null;
  const showInstanceEditor = section === "instances" && (selectedInstance !== null || isNewInstance);
  const selectedDescriptor = selectedMw
    ? descriptors.find((d) => d.name === selectedMw)
    : null;
  const instanceTypeDescriptor = editInstanceType
    ? descriptors.find((d) => d.name === editInstanceType)
    : null;

  return (
    <div className="flex-1 flex flex-col min-h-0">
      <ViewHeader title="Middleware" subtitle="Presets, global configuration, and named instances" />
      <div className="flex-1 flex min-h-0">
      {/* Left sidebar */}
      <div className="w-72 border-r border-gray-200 overflow-y-auto">
        {/* Presets section */}
        <div className="px-4 py-3 border-b border-gray-200 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-gray-800">Presets</h2>
          <button
            onClick={startNewPreset}
            className="flex items-center gap-1 px-2 py-1 text-xs text-blue-600 hover:bg-blue-50 rounded"
          >
            <Plus size={14} />
            New
          </button>
        </div>
        <div className="divide-y divide-gray-100">
          {Object.entries(presets).map(([name, mws]) => (
            <button
              key={name}
              onClick={() => selectPreset(name)}
              className={`w-full text-left px-4 py-2.5 hover:bg-gray-50 ${
                section === "presets" && selectedPreset === name && !isNewPreset
                  ? "bg-blue-50"
                  : ""
              }`}
            >
              <div className="text-sm font-medium text-gray-800">{name}</div>
              <div className="text-xs text-gray-400 truncate">{mws.join(", ")}</div>
            </button>
          ))}
          {Object.keys(presets).length === 0 && (
            <div className="p-4 text-xs text-gray-400">No presets defined.</div>
          )}
        </div>

        {/* Config section */}
        <div className="px-4 py-3 border-b border-t border-gray-200">
          <h2 className="text-sm font-semibold text-gray-800">Configuration</h2>
        </div>
        <div className="divide-y divide-gray-100">
          {configurableMiddleware.map((d) => (
            <button
              key={d.name}
              onClick={() => selectConfig(d.name)}
              className={`w-full text-left px-4 py-2.5 hover:bg-gray-50 ${
                section === "config" && selectedMw === d.name ? "bg-blue-50" : ""
              }`}
            >
              <div className="text-sm font-medium text-gray-800">{d.name}</div>
              {d.description && (
                <div className="text-xs text-gray-400">{d.description}</div>
              )}
              <div className="text-xs text-gray-400">
                {d.config_fields.length} field{d.config_fields.length !== 1 ? "s" : ""}
                {mwConfig[d.name] ? " \u00b7 configured" : ""}
              </div>
            </button>
          ))}
        </div>

        {/* Instances section */}
        <div className="px-4 py-3 border-b border-t border-gray-200 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-gray-800">Instances</h2>
          <button
            onClick={startNewInstance}
            className="flex items-center gap-1 px-2 py-1 text-xs text-blue-600 hover:bg-blue-50 rounded"
          >
            <Plus size={14} />
            New
          </button>
        </div>
        <div className="divide-y divide-gray-100">
          {Object.entries(instances).map(([key, inst]) => (
            <button
              key={key}
              onClick={() => selectInstance(key)}
              className={`w-full text-left px-4 py-2.5 hover:bg-gray-50 ${
                section === "instances" && selectedInstance === key && !isNewInstance
                  ? "bg-blue-50"
                  : ""
              }`}
            >
              <div className="text-sm font-medium text-gray-800">{key}</div>
              <div className="text-xs text-gray-400">type: {inst.type}</div>
            </button>
          ))}
          {Object.keys(instances).length === 0 && (
            <div className="p-4 text-xs text-gray-400">No instances defined.</div>
          )}
        </div>
      </div>

      {/* Right panel */}
      <div className="flex-1 overflow-y-auto p-6">
        {showPresetEditor ? (
          <div className="max-w-2xl space-y-5">
            <div className="flex items-center justify-between">
              <h3 className="text-lg font-semibold text-gray-900">
                {isNewPreset ? "New Preset" : editPresetName}
              </h3>
              <div className="flex items-center gap-2">
                {!isNewPreset && (
                  <button
                    onClick={deletePreset}
                    className="px-3 py-1.5 text-sm text-red-600 border border-red-300 rounded hover:bg-red-50"
                  >
                    <Trash2 size={14} className="inline mr-1" />
                    Delete
                  </button>
                )}
                <button
                  onClick={savePreset}
                  disabled={saving || !editPresetName}
                  className="px-4 py-1.5 text-sm text-white bg-blue-500 rounded hover:bg-blue-600 disabled:opacity-50"
                >
                  {saving ? "Saving..." : "Save"}
                </button>
              </div>
            </div>

            <FieldLabel label="Name">
              <input
                type="text"
                value={editPresetName}
                onChange={(e) => setEditPresetName(e.target.value)}
                className="input-field font-mono"
                placeholder="e.g. authenticated"
              />
            </FieldLabel>

            <FieldLabel label="Middleware">
              <div className="flex flex-wrap gap-1.5 mb-1.5">
                {editPresetMws.map((mw) => (
                  <span
                    key={mw}
                    className="inline-flex items-center gap-1 px-2 py-0.5 text-xs bg-gray-100 text-gray-700 rounded"
                  >
                    {mw}
                    <button
                      type="button"
                      onClick={() => setEditPresetMws(editPresetMws.filter((x) => x !== mw))}
                      className="text-gray-400 hover:text-gray-600"
                    >
                      <X size={10} />
                    </button>
                  </span>
                ))}
              </div>
              <select
                value=""
                onChange={(e) => {
                  const val = e.target.value;
                  if (val && !editPresetMws.includes(val)) {
                    setEditPresetMws([...editPresetMws, val]);
                  }
                }}
                className="input-field"
              >
                <option value="">Add middleware...</option>
                {descriptors
                  .map((d) => d.name)
                  .filter((n) => !editPresetMws.includes(n))
                  .map((n) => (
                    <option key={n} value={n}>{n}</option>
                  ))}
                {Object.keys(instances).filter((n) => !editPresetMws.includes(n)).length > 0 && (
                  <optgroup label="Instances">
                    {Object.keys(instances)
                      .filter((n) => !editPresetMws.includes(n))
                      .map((n) => (
                        <option key={n} value={n}>{n}</option>
                      ))}
                  </optgroup>
                )}
              </select>
            </FieldLabel>
          </div>
        ) : showConfigEditor && selectedDescriptor ? (
          <div className="max-w-2xl space-y-5">
            <div className="flex items-center justify-between">
              <div>
                <h3 className="text-lg font-semibold text-gray-900">{selectedMw}</h3>
                {selectedDescriptor?.description && (
                  <p className="text-xs text-gray-400 mt-0.5">{selectedDescriptor.description}</p>
                )}
              </div>
              <div className="flex items-center gap-2">
                {selectedMw && mwConfig[selectedMw] && (
                  <button
                    onClick={() => setShowResolved(!showResolved)}
                    className={`flex items-center gap-1 px-2.5 py-1.5 text-xs rounded border transition-colors ${
                      showResolved
                        ? "bg-amber-50 border-amber-300 text-amber-700"
                        : "border-gray-300 text-gray-500 hover:bg-gray-50"
                    }`}
                    title={showResolved ? "Showing resolved values" : "Show resolved values"}
                  >
                    {showResolved ? <Eye size={13} /> : <EyeOff size={13} />}
                    Resolved
                  </button>
                )}
                <button
                  onClick={saveConfig}
                  disabled={saving || showResolved}
                  className="px-4 py-1.5 text-sm text-white bg-blue-500 rounded hover:bg-blue-600 disabled:opacity-50"
                >
                  {saving ? "Saving..." : "Save"}
                </button>
              </div>
            </div>

            {showResolved && selectedMw && mwConfig[selectedMw] && (
              <div className="text-xs text-amber-600 bg-amber-50 border border-amber-200 rounded px-3 py-2">
                Showing resolved values (read-only). Expressions and variables have been evaluated.
              </div>
            )}

            {selectedDescriptor.config_fields.map((field) => {
              const resolvedCfg = selectedMw ? mwConfig[selectedMw] : undefined;
              const displayValue = showResolved && resolvedCfg
                ? resolvedCfg[field.key]
                : editConfig[field.key];
              return (
                <ConfigFieldInput
                  key={field.key}
                  field={field}
                  value={displayValue}
                  onChange={(val) => setEditConfig({ ...editConfig, [field.key]: val })}
                  readOnly={showResolved}
                />
              );
            })}
          </div>
        ) : showInstanceEditor ? (
          <div className="max-w-2xl space-y-5">
            <div className="flex items-center justify-between">
              <h3 className="text-lg font-semibold text-gray-900">
                {isNewInstance ? "New Instance" : selectedInstance}
              </h3>
              <div className="flex items-center gap-2">
                {!isNewInstance && selectedInstance && instances[selectedInstance] && (
                  <button
                    onClick={() => setShowResolved(!showResolved)}
                    className={`flex items-center gap-1 px-2.5 py-1.5 text-xs rounded border transition-colors ${
                      showResolved
                        ? "bg-amber-50 border-amber-300 text-amber-700"
                        : "border-gray-300 text-gray-500 hover:bg-gray-50"
                    }`}
                    title={showResolved ? "Showing resolved values" : "Show resolved values"}
                  >
                    {showResolved ? <Eye size={13} /> : <EyeOff size={13} />}
                    Resolved
                  </button>
                )}
                {!isNewInstance && (
                  <button
                    onClick={deleteInstance}
                    className="px-3 py-1.5 text-sm text-red-600 border border-red-300 rounded hover:bg-red-50"
                  >
                    <Trash2 size={14} className="inline mr-1" />
                    Delete
                  </button>
                )}
                <button
                  onClick={saveInstance}
                  disabled={saving || showResolved || !editInstanceName || !editInstanceType}
                  className="px-4 py-1.5 text-sm text-white bg-blue-500 rounded hover:bg-blue-600 disabled:opacity-50"
                >
                  {saving ? "Saving..." : "Save"}
                </button>
              </div>
            </div>

            {showResolved && selectedInstance && instances[selectedInstance] && (
              <div className="text-xs text-amber-600 bg-amber-50 border border-amber-200 rounded px-3 py-2">
                Showing resolved values (read-only). Expressions and variables have been evaluated.
              </div>
            )}

            <FieldLabel label="Type">
              <select
                value={showResolved && selectedInstance ? (instances[selectedInstance]?.type ?? editInstanceType) : editInstanceType}
                onChange={(e) => {
                  setEditInstanceType(e.target.value);
                  setEditInstanceConfig({});
                }}
                className="input-field"
                disabled={showResolved}
              >
                <option value="">Select type...</option>
                {descriptors
                  .filter((d) => d.config_fields.length > 0)
                  .map((d) => (
                    <option key={d.name} value={d.name}>{d.name}</option>
                  ))}
              </select>
            </FieldLabel>

            <FieldLabel label="Name">
              <input
                type="text"
                value={editInstanceName}
                onChange={(e) => setEditInstanceName(e.target.value)}
                className="input-field font-mono"
                placeholder="e.g. v1, strict, tenant"
                readOnly={showResolved}
              />
              {editInstanceType && editInstanceName && (
                <div className="text-xs text-gray-400 mt-1">
                  Full key: <span className="font-mono">{editInstanceType}:{editInstanceName}</span>
                </div>
              )}
            </FieldLabel>

            {instanceTypeDescriptor && instanceTypeDescriptor.config_fields.length > 0 && (
              <>
                <div className="border-t border-gray-200 pt-4">
                  <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-3">
                    Configuration
                  </h4>
                </div>
                {instanceTypeDescriptor.config_fields.map((field) => {
                  const resolvedInst = selectedInstance ? instances[selectedInstance] : undefined;
                  const displayValue = showResolved && resolvedInst
                    ? resolvedInst.config[field.key]
                    : editInstanceConfig[field.key];
                  return (
                    <ConfigFieldInput
                      key={field.key}
                      field={field}
                      value={displayValue}
                      onChange={(val) =>
                        setEditInstanceConfig({ ...editInstanceConfig, [field.key]: val })
                      }
                      readOnly={showResolved}
                    />
                  );
                })}
              </>
            )}
          </div>
        ) : (
          <div className="text-sm text-gray-400">
            Select a preset, middleware, or instance to configure.
          </div>
        )}
      </div>
      </div>
    </div>
  );
}

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

function ConfigFieldInput({
  field,
  value,
  onChange,
  readOnly,
}: {
  field: ConfigField;
  value: unknown;
  onChange: (value: unknown) => void;
  readOnly?: boolean;
}) {
  const label = field.key.replace(/_/g, " ");
  const roClass = readOnly ? " bg-gray-50 text-gray-500" : "";

  switch (field.type) {
    case "string":
      return (
        <FieldLabel label={label}>
          <input
            type="text"
            value={(value as string) ?? ""}
            onChange={(e) => onChange(e.target.value || undefined)}
            className={"input-field font-mono" + roClass}
            placeholder={field.placeholder}
            readOnly={readOnly}
          />
          {!readOnly && field.required && !value && (
            <span className="text-xs text-red-400 mt-0.5 block">Required</span>
          )}
        </FieldLabel>
      );
    case "number":
      return (
        <FieldLabel label={label}>
          <input
            type="number"
            value={(value as number) ?? ""}
            onChange={(e) => {
              const n = e.target.value ? Number(e.target.value) : undefined;
              onChange(n);
            }}
            className={"input-field" + roClass}
            placeholder={field.placeholder}
            readOnly={readOnly}
          />
        </FieldLabel>
      );
    case "boolean":
      return (
        <div>
          <label className="inline-flex items-center gap-2 text-sm text-gray-600 cursor-pointer">
            <input
              type="checkbox"
              checked={!!value}
              onChange={(e) => onChange(e.target.checked || undefined)}
              className="rounded border-gray-300"
              disabled={readOnly}
            />
            <span className="text-xs font-medium text-gray-400 uppercase">{label}</span>
          </label>
        </div>
      );
    case "select":
      return (
        <FieldLabel label={label}>
          <select
            value={(value as string) ?? (field.default as string) ?? ""}
            onChange={(e) => onChange(e.target.value || undefined)}
            className={"input-field" + roClass}
            disabled={readOnly}
          >
            <option value="">Select...</option>
            {(field.options ?? []).map((opt) => (
              <option key={opt} value={opt}>{opt}</option>
            ))}
          </select>
        </FieldLabel>
      );
    case "text":
      return (
        <FieldLabel label={label}>
          <textarea
            value={(value as string) ?? ""}
            onChange={(e) => onChange(e.target.value || undefined)}
            className={"input-field font-mono h-32 resize-y" + roClass}
            placeholder={field.placeholder}
            readOnly={readOnly}
          />
          {!readOnly && field.required && !value && (
            <span className="text-xs text-red-400 mt-0.5 block">Required</span>
          )}
        </FieldLabel>
      );
  }
}
