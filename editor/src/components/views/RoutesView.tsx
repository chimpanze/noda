import { useEffect, useState, useCallback } from "react";
import { ExternalLink } from "lucide-react";
import * as api from "@/api/client";
import { useEditorStore } from "@/stores/editor";

interface RouteConfig {
  id: string;
  method: string;
  path: string;
  summary?: string;
  tags?: string[];
  middleware?: string[];
  trigger?: {
    workflow: string;
    input?: Record<string, string>;
  };
  [key: string]: unknown;
}

const methodColors: Record<string, string> = {
  GET: "bg-green-100 text-green-800",
  POST: "bg-blue-100 text-blue-800",
  PUT: "bg-yellow-100 text-yellow-800",
  PATCH: "bg-orange-100 text-orange-800",
  DELETE: "bg-red-100 text-red-800",
  HEAD: "bg-gray-100 text-gray-700",
  OPTIONS: "bg-purple-100 text-purple-800",
};

export function RoutesView() {
  const files = useEditorStore((s) => s.files);
  const setActiveView = useEditorStore((s) => s.setActiveView);
  const setActiveWorkflow = useEditorStore((s) => s.setActiveWorkflow);

  const [routeFiles, setRouteFiles] = useState<{ path: string; routes: RouteConfig[] }[]>([]);
  const [selectedRoute, setSelectedRoute] = useState<{ filePath: string; route: RouteConfig } | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!files?.routes) return;
    setLoading(true);
    Promise.all(
      files.routes.map(async (path) => {
        const data = await api.readFile(path);
        const routes = Array.isArray(data) ? (data as RouteConfig[]) : [data as RouteConfig];
        return { path, routes };
      })
    )
      .then(setRouteFiles)
      .finally(() => setLoading(false));
  }, [files?.routes]);

  const allRoutes = routeFiles.flatMap((f) =>
    f.routes.map((r) => ({ filePath: f.path, route: r }))
  );

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

  if (loading) {
    return <div className="p-6 text-sm text-gray-400">Loading routes...</div>;
  }

  return (
    <div className="flex-1 flex min-h-0">
      {/* Route list */}
      <div className="w-96 border-r border-gray-200 overflow-y-auto">
        <div className="px-4 py-3 border-b border-gray-200 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-gray-800">Routes ({allRoutes.length})</h2>
        </div>
        <div className="divide-y divide-gray-100">
          {allRoutes.map(({ filePath, route }) => (
            <button
              key={`${filePath}-${route.id}`}
              onClick={() => setSelectedRoute({ filePath, route })}
              className={`w-full text-left px-4 py-2.5 hover:bg-gray-50 flex items-center gap-3 ${
                selectedRoute?.route.id === route.id ? "bg-blue-50" : ""
              }`}
            >
              <span
                className={`px-2 py-0.5 text-xs font-mono font-medium rounded ${
                  methodColors[route.method] ?? "bg-gray-100 text-gray-700"
                }`}
              >
                {route.method}
              </span>
              <div className="flex-1 min-w-0">
                <div className="text-sm font-medium text-gray-800 truncate">{route.path}</div>
                {route.summary && (
                  <div className="text-xs text-gray-400 truncate">{route.summary}</div>
                )}
              </div>
            </button>
          ))}
          {allRoutes.length === 0 && (
            <div className="p-4 text-sm text-gray-400">No routes configured.</div>
          )}
        </div>
      </div>

      {/* Route detail */}
      <div className="flex-1 overflow-y-auto p-6">
        {selectedRoute ? (
          <RouteDetail route={selectedRoute.route} onGoToWorkflow={goToWorkflow} />
        ) : (
          <div className="text-sm text-gray-400">Select a route to view details.</div>
        )}
      </div>
    </div>
  );
}

function RouteDetail({
  route,
  onGoToWorkflow,
}: {
  route: RouteConfig;
  onGoToWorkflow: (id: string) => void;
}) {
  return (
    <div className="max-w-2xl space-y-6">
      <div>
        <h3 className="text-lg font-semibold text-gray-900">{route.id}</h3>
        {route.summary && <p className="text-sm text-gray-500 mt-1">{route.summary}</p>}
      </div>

      {/* Method + Path */}
      <div className="grid grid-cols-[100px_1fr] gap-3">
        <div>
          <label className="text-xs font-medium text-gray-400 uppercase">Method</label>
          <div className="mt-1">
            <span
              className={`px-2 py-1 text-sm font-mono font-medium rounded ${
                methodColors[route.method] ?? "bg-gray-100"
              }`}
            >
              {route.method}
            </span>
          </div>
        </div>
        <div>
          <label className="text-xs font-medium text-gray-400 uppercase">Path</label>
          <div className="mt-1 text-sm font-mono text-gray-800 bg-gray-50 px-3 py-1.5 rounded">
            {route.path}
          </div>
        </div>
      </div>

      {/* Trigger */}
      {route.trigger && (
        <div>
          <label className="text-xs font-medium text-gray-400 uppercase">Trigger Workflow</label>
          <div className="mt-1 flex items-center gap-2">
            <span className="text-sm font-medium text-blue-700">{route.trigger.workflow}</span>
            <button
              onClick={() => onGoToWorkflow(route.trigger!.workflow)}
              className="text-blue-500 hover:text-blue-700"
              title="Open workflow"
            >
              <ExternalLink size={14} />
            </button>
          </div>
          {route.trigger.input && Object.keys(route.trigger.input).length > 0 && (
            <div className="mt-2">
              <label className="text-xs text-gray-400">Input Mapping</label>
              <div className="mt-1 space-y-1">
                {Object.entries(route.trigger.input).map(([key, val]) => (
                  <div key={key} className="flex gap-2 text-xs font-mono">
                    <span className="text-gray-600 w-28 shrink-0">{key}:</span>
                    <span className="text-purple-600">{val}</span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      {/* Middleware */}
      {route.middleware && route.middleware.length > 0 && (
        <div>
          <label className="text-xs font-medium text-gray-400 uppercase">Middleware</label>
          <div className="mt-1 flex flex-wrap gap-1.5">
            {route.middleware.map((mw) => (
              <span key={mw} className="px-2 py-0.5 text-xs bg-gray-100 text-gray-700 rounded">
                {mw}
              </span>
            ))}
          </div>
        </div>
      )}

      {/* Tags */}
      {route.tags && route.tags.length > 0 && (
        <div>
          <label className="text-xs font-medium text-gray-400 uppercase">Tags</label>
          <div className="mt-1 flex flex-wrap gap-1.5">
            {route.tags.map((tag) => (
              <span key={tag} className="px-2 py-0.5 text-xs bg-blue-50 text-blue-700 rounded">
                {tag}
              </span>
            ))}
          </div>
        </div>
      )}

      {/* Raw JSON */}
      <div>
        <label className="text-xs font-medium text-gray-400 uppercase">JSON</label>
        <pre className="mt-1 p-3 bg-gray-50 rounded text-xs text-gray-600 overflow-x-auto whitespace-pre-wrap border border-gray-200">
          {JSON.stringify(route, null, 2)}
        </pre>
      </div>
    </div>
  );
}
