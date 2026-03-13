import { useEffect, useState, useCallback } from "react";
import {
  X,
  ChevronDown,
  ChevronRight,
  AlertTriangle,
  CheckCircle,
} from "lucide-react";
import { ViewHeader } from "@/components/layout/ViewHeader";
import Editor from "@monaco-editor/react";
import * as api from "@/api/client";
import type { EnvVarInfo } from "@/api/client";
import { useEditorStore } from "@/stores/editor";
import { showToast } from "@/utils/toast";

interface ServerConfig {
  port?: number;
  read_timeout?: string;
  write_timeout?: string;
  response_timeout?: string;
  body_limit?: number;
}

export function ServerSettingsView() {
  const files = useEditorStore((s) => s.files);

  const [rootConfig, setRootConfig] = useState<Record<string, unknown>>({});
  const [rootPath, setRootPath] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  // Server fields
  const [server, setServer] = useState<ServerConfig>({});

  // Global middleware
  const [globalMiddleware, setGlobalMiddleware] = useState<string[]>([]);
  const [middlewareNames, setMiddlewareNames] = useState<string[]>([]);

  // Environment
  const [envVars, setEnvVars] = useState<EnvVarInfo[]>([]);
  const [envFileContent, setEnvFileContent] = useState<string | null>(null);
  const [envFileExists, setEnvFileExists] = useState(false);
  const [envExpanded, setEnvExpanded] = useState(false);
  const [savingEnv, setSavingEnv] = useState(false);

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      const [rootData, mwInfo] = await Promise.all([
        files?.root
          ? (api.readFile(files.root) as Promise<Record<string, unknown>>)
          : Promise.resolve({} as Record<string, unknown>),
        api.listMiddleware(),
      ]);
      setRootConfig(rootData);
      if (files?.root) setRootPath(files.root);

      const srv = (rootData.server ?? {}) as ServerConfig;
      setServer({ ...srv });
      setGlobalMiddleware(
        Array.isArray(rootData.global_middleware)
          ? [...(rootData.global_middleware as string[])]
          : [],
      );

      const instanceNames = Object.keys(mwInfo.instances ?? {});
      setMiddlewareNames([
        ...mwInfo.middleware.map((m) => m.name),
        ...instanceNames,
      ]);

      // Load env vars
      try {
        const vars = await api.getEnvVars();
        setEnvVars(vars);
      } catch {
        // env endpoint may not exist in production
      }

      // Try to load .env file
      try {
        const envData = await api.readFile(".env");
        setEnvFileContent(
          typeof envData === "string" ? envData : JSON.stringify(envData),
        );
        setEnvFileExists(true);
      } catch {
        setEnvFileContent(null);
        setEnvFileExists(false);
      }
    } finally {
      setLoading(false);
    }
  }, [files?.root]);

  useEffect(() => {
    reload();
  }, [reload]);

  const handleSave = useCallback(async () => {
    if (!rootPath) return;
    setSaving(true);
    try {
      const updated = structuredClone(rootConfig);

      // Server — omit empty/default values
      const cleanServer: Record<string, unknown> = {};
      if (server.port) cleanServer.port = server.port;
      if (server.read_timeout) cleanServer.read_timeout = server.read_timeout;
      if (server.write_timeout)
        cleanServer.write_timeout = server.write_timeout;
      if (server.response_timeout)
        cleanServer.response_timeout = server.response_timeout;
      if (server.body_limit) cleanServer.body_limit = server.body_limit;

      if (Object.keys(cleanServer).length > 0) {
        updated.server = cleanServer;
      } else {
        delete updated.server;
      }

      // Global middleware
      if (globalMiddleware.length > 0) {
        updated.global_middleware = globalMiddleware;
      } else {
        delete updated.global_middleware;
      }

      await api.writeFile(rootPath, updated);
      showToast({ type: "success", message: "Settings saved" });
      await reload();
    } catch (err) {
      showToast({ type: "error", message: `Failed to save settings: ${err}` });
    } finally {
      setSaving(false);
    }
  }, [rootPath, rootConfig, server, globalMiddleware, reload]);

  if (loading) {
    return (
      <div className="flex-1 flex flex-col min-h-0">
        <ViewHeader
          title="Settings"
          subtitle="Server configuration and environment"
        />
        <div className="p-6 text-sm text-gray-400">Loading settings...</div>
      </div>
    );
  }

  return (
    <div className="flex-1 flex flex-col min-h-0">
      <ViewHeader
        title="Settings"
        subtitle="Server configuration and environment"
      />
      <div className="flex-1 overflow-y-auto p-6">
        <div className="max-w-2xl mx-auto space-y-6">
          <div className="flex items-center justify-between">
            <h3 className="text-lg font-semibold text-gray-900">
              Server Settings
            </h3>
            <button
              onClick={handleSave}
              disabled={saving}
              className="px-4 py-1.5 text-sm text-white bg-blue-500 rounded hover:bg-blue-600 disabled:opacity-50"
            >
              {saving ? "Saving..." : "Save"}
            </button>
          </div>

          {/* Server section */}
          <div>
            <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-3">
              Server
            </h4>
            <div className="space-y-4">
              <FieldLabel label="Port">
                <input
                  type="number"
                  value={server.port ?? ""}
                  onChange={(e) =>
                    setServer({
                      ...server,
                      port: e.target.value ? Number(e.target.value) : undefined,
                    })
                  }
                  className="input-field"
                  placeholder="3000"
                />
              </FieldLabel>

              <div className="grid grid-cols-2 gap-3">
                <FieldLabel label="Read Timeout">
                  <input
                    type="text"
                    value={server.read_timeout ?? ""}
                    onChange={(e) =>
                      setServer({
                        ...server,
                        read_timeout: e.target.value || undefined,
                      })
                    }
                    className="input-field font-mono"
                    placeholder="10s"
                  />
                </FieldLabel>
                <FieldLabel label="Write Timeout">
                  <input
                    type="text"
                    value={server.write_timeout ?? ""}
                    onChange={(e) =>
                      setServer({
                        ...server,
                        write_timeout: e.target.value || undefined,
                      })
                    }
                    className="input-field font-mono"
                    placeholder="10s"
                  />
                </FieldLabel>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <FieldLabel label="Response Timeout">
                  <input
                    type="text"
                    value={server.response_timeout ?? ""}
                    onChange={(e) =>
                      setServer({
                        ...server,
                        response_timeout: e.target.value || undefined,
                      })
                    }
                    className="input-field font-mono"
                    placeholder="30s"
                  />
                </FieldLabel>
                <FieldLabel label="Body Limit">
                  <input
                    type="number"
                    value={server.body_limit ?? ""}
                    onChange={(e) =>
                      setServer({
                        ...server,
                        body_limit: e.target.value
                          ? Number(e.target.value)
                          : undefined,
                      })
                    }
                    className="input-field"
                    placeholder="4194304"
                  />
                </FieldLabel>
              </div>
            </div>
          </div>

          {/* Global Middleware section */}
          <div className="border-t border-gray-200 pt-4">
            <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-3">
              Global Middleware
            </h4>
            <div className="flex flex-wrap gap-1.5 mb-1.5">
              {globalMiddleware.map((mw) => (
                <span
                  key={mw}
                  className="inline-flex items-center gap-1 px-2 py-0.5 text-xs bg-gray-100 text-gray-700 rounded"
                >
                  {mw}
                  <button
                    type="button"
                    onClick={() =>
                      setGlobalMiddleware(
                        globalMiddleware.filter((x) => x !== mw),
                      )
                    }
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
                if (val && !globalMiddleware.includes(val)) {
                  setGlobalMiddleware([...globalMiddleware, val]);
                }
              }}
              className="input-field"
            >
              <option value="">Add middleware...</option>
              {middlewareNames
                .filter((n) => !globalMiddleware.includes(n))
                .map((n) => (
                  <option key={n} value={n}>
                    {n}
                  </option>
                ))}
            </select>
          </div>
          {/* Environment Variables */}
          {envVars.length > 0 && (
            <div className="border-t border-gray-200 pt-4">
              <button
                onClick={() => setEnvExpanded(!envExpanded)}
                className="flex items-center gap-2 w-full text-left"
              >
                {envExpanded ? (
                  <ChevronDown size={14} className="text-gray-400" />
                ) : (
                  <ChevronRight size={14} className="text-gray-400" />
                )}
                <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider">
                  Environment Variables ({envVars.length})
                </h4>
                {envVars.some((v) => !v.defined) && (
                  <span className="flex items-center gap-1 text-xs text-amber-600">
                    <AlertTriangle size={12} />
                    {envVars.filter((v) => !v.defined).length} undefined
                  </span>
                )}
              </button>

              {envExpanded && (
                <div className="mt-3 space-y-4">
                  {/* Undefined vars warning */}
                  {envVars.some((v) => !v.defined) && (
                    <div className="p-2 bg-amber-50 border border-amber-200 rounded text-xs text-amber-800">
                      Some environment variables referenced in config are not
                      defined. Set them in your environment or .env file.
                    </div>
                  )}

                  {/* Env var table */}
                  <div className="border border-gray-200 rounded overflow-hidden">
                    <table className="w-full text-xs">
                      <thead>
                        <tr className="bg-gray-50 border-b border-gray-200">
                          <th className="text-left px-3 py-1.5 font-medium text-gray-500">
                            Variable
                          </th>
                          <th className="text-left px-3 py-1.5 font-medium text-gray-500 w-20">
                            Status
                          </th>
                          <th className="text-left px-3 py-1.5 font-medium text-gray-500">
                            Source
                          </th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-gray-100">
                        {envVars.map((v) => (
                          <tr
                            key={v.name}
                            className={v.defined ? "" : "bg-amber-50/50"}
                          >
                            <td className="px-3 py-1.5 font-mono text-gray-800">
                              {v.name}
                            </td>
                            <td className="px-3 py-1.5">
                              {v.defined ? (
                                <span className="flex items-center gap-1 text-green-600">
                                  <CheckCircle size={10} /> Set
                                </span>
                              ) : (
                                <span className="flex items-center gap-1 text-amber-600">
                                  <AlertTriangle size={10} /> Missing
                                </span>
                              )}
                            </td>
                            <td className="px-3 py-1.5 text-gray-500 font-mono">
                              {v.sources.join(", ")}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>

                  {/* .env file editor */}
                  <div>
                    <div className="flex items-center justify-between mb-2">
                      <label className="text-xs font-medium text-gray-400 uppercase">
                        .env File
                      </label>
                      {envFileExists && (
                        <button
                          onClick={async () => {
                            if (envFileContent === null) return;
                            setSavingEnv(true);
                            try {
                              // Write raw text as JSON string
                              await api.writeFile(".env", envFileContent);
                              showToast({
                                type: "success",
                                message: ".env file saved",
                              });
                              await reload();
                            } catch (err) {
                              showToast({
                                type: "error",
                                message: `Failed to save .env: ${err}`,
                              });
                            } finally {
                              setSavingEnv(false);
                            }
                          }}
                          disabled={savingEnv}
                          className="px-2 py-1 text-xs text-blue-500 hover:text-blue-700 hover:bg-blue-50 rounded disabled:opacity-50"
                        >
                          {savingEnv ? "Saving..." : "Save .env"}
                        </button>
                      )}
                    </div>
                    {envFileExists ? (
                      <div className="border border-gray-200 rounded overflow-hidden">
                        <Editor
                          height="200px"
                          language="ini"
                          value={envFileContent ?? ""}
                          onChange={(v) => setEnvFileContent(v ?? "")}
                          options={{
                            minimap: { enabled: false },
                            fontSize: 12,
                            scrollBeyondLastLine: false,
                            lineNumbers: "on",
                            wordWrap: "on",
                          }}
                        />
                      </div>
                    ) : (
                      <button
                        onClick={async () => {
                          try {
                            const template = envVars
                              .filter((v) => !v.defined)
                              .map((v) => `${v.name}=`)
                              .join("\n");
                            await api.writeFile(
                              ".env",
                              template || "# Environment variables\n",
                            );
                            showToast({
                              type: "success",
                              message: ".env file created",
                            });
                            setEnvFileContent(
                              template || "# Environment variables\n",
                            );
                            setEnvFileExists(true);
                          } catch (err) {
                            showToast({
                              type: "error",
                              message: `Failed to create .env: ${err}`,
                            });
                          }
                        }}
                        className="px-3 py-1.5 text-xs text-blue-600 border border-blue-300 rounded hover:bg-blue-50"
                      >
                        Create .env File
                      </button>
                    )}
                  </div>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function FieldLabel({
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
