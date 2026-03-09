import { useEffect, useState } from "react";
import { CheckCircle, XCircle, HelpCircle, RefreshCw } from "lucide-react";
import * as api from "@/api/client";
import type { ServiceInfo, PluginInfo } from "@/types";

export function ServicesView() {
  const [services, setServices] = useState<ServiceInfo[]>([]);
  const [plugins, setPlugins] = useState<PluginInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);

  const loadData = async () => {
    try {
      const [svc, plg] = await Promise.all([api.listServices(), api.listPlugins()]);
      setServices(svc);
      setPlugins(plg);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  };

  useEffect(() => {
    loadData();
    const interval = setInterval(loadData, 30000);
    return () => clearInterval(interval);
  }, []);

  const refresh = () => {
    setRefreshing(true);
    loadData();
  };

  // Group services by prefix
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
          <h2 className="text-lg font-semibold text-gray-900">Services</h2>
          <button
            onClick={refresh}
            disabled={refreshing}
            className="flex items-center gap-1.5 text-xs text-gray-500 hover:text-gray-700 disabled:opacity-50"
          >
            <RefreshCw size={14} className={refreshing ? "animate-spin" : ""} />
            Refresh
          </button>
        </div>

        {/* Plugins summary */}
        <div className="mb-6 grid grid-cols-3 gap-3">
          {plugins.map((p) => (
            <div key={p.prefix} className="border border-gray-200 rounded-lg p-3">
              <div className="text-sm font-medium text-gray-800">{p.name}</div>
              <div className="text-xs text-gray-400 mt-0.5">
                {p.prefix}.* · {p.node_count} nodes
              </div>
            </div>
          ))}
        </div>

        {/* Service instances */}
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
          <div className="text-sm text-gray-400">No services configured.</div>
        )}
      </div>
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
