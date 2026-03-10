import { useEffect, useState, useCallback } from "react";
import { ExternalLink, Plus } from "lucide-react";
import * as api from "@/api/client";
import { useEditorStore } from "@/stores/editor";
import { RouteFormPanel, type RouteConfig } from "./RouteFormPanel";
import { showToast } from "@/components/panels/Toast";

const methodColors: Record<string, string> = {
  GET: "bg-green-100 text-green-800",
  POST: "bg-blue-100 text-blue-800",
  PUT: "bg-yellow-100 text-yellow-800",
  PATCH: "bg-orange-100 text-orange-800",
  DELETE: "bg-red-100 text-red-800",
  HEAD: "bg-gray-100 text-gray-700",
  OPTIONS: "bg-purple-100 text-purple-800",
};

interface RouteFileEntry {
  filePath: string;
  route: RouteConfig;
}

export function RoutesView() {
  const files = useEditorStore((s) => s.files);
  const loadFiles = useEditorStore((s) => s.loadFiles);
  const setActiveView = useEditorStore((s) => s.setActiveView);
  const setActiveWorkflow = useEditorStore((s) => s.setActiveWorkflow);

  const [routeEntries, setRouteEntries] = useState<RouteFileEntry[]>([]);
  const [selectedIndex, setSelectedIndex] = useState<number | null>(null);
  const [editRoute, setEditRoute] = useState<RouteConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [isNew, setIsNew] = useState(false);

  const reload = useCallback(async () => {
    if (!files?.routes) return;
    setLoading(true);
    try {
      const entries: RouteFileEntry[] = [];
      await Promise.all(
        files.routes.map(async (path) => {
          const data = await api.readFile(path);
          const routes = Array.isArray(data)
            ? (data as RouteConfig[])
            : [data as RouteConfig];
          for (const route of routes) {
            entries.push({ filePath: path, route });
          }
        })
      );
      setRouteEntries(entries);
    } finally {
      setLoading(false);
    }
  }, [files?.routes]);

  useEffect(() => {
    reload();
  }, [reload]);

  const selectRoute = useCallback(
    (index: number) => {
      setSelectedIndex(index);
      setEditRoute(structuredClone(routeEntries[index].route));
      setIsNew(false);
    },
    [routeEntries]
  );

  const startNew = useCallback(() => {
    setSelectedIndex(null);
    setIsNew(true);
    setEditRoute({
      id: "",
      method: "GET",
      path: "/api/",
      trigger: { workflow: "" },
    });
  }, []);

  const handleSave = useCallback(async () => {
    if (!editRoute || !editRoute.id) return;
    setSaving(true);
    try {
      // Clean up empty optional fields before saving
      const clean = { ...editRoute };
      if (!clean.summary) delete clean.summary;
      if (!clean.tags?.length) delete clean.tags;
      if (!clean.middleware?.length) delete clean.middleware;
      if (!clean.body || (!clean.body.schema && !clean.body.raw)) delete clean.body;
      if (clean.trigger) {
        if (!clean.trigger.workflow) delete (clean as Record<string, unknown>).trigger;
        else {
          if (!clean.trigger.input || Object.keys(clean.trigger.input).length === 0)
            delete clean.trigger.input;
          if (!clean.trigger.files?.length) delete clean.trigger.files;
        }
      }

      if (isNew) {
        // Create new route file
        const filePath = `routes/${clean.id}.json`;
        await api.writeFile(filePath, clean);
        showToast({ type: "success", message: `Route "${clean.id}" created` });
      } else if (selectedIndex !== null) {
        // Update existing route file
        const entry = routeEntries[selectedIndex];
        await api.writeFile(entry.filePath, clean);
        showToast({ type: "success", message: `Route "${clean.id}" saved` });
      }

      await loadFiles();
      await reload();
      setIsNew(false);
      // Re-select the saved route
      const newEntries = routeEntries; // Will be stale, but reload will refresh
      const idx = newEntries.findIndex((e) => e.route.id === clean.id);
      if (idx >= 0) setSelectedIndex(idx);
    } catch (err) {
      showToast({ type: "error", message: `Failed to save route: ${err}` });
    } finally {
      setSaving(false);
    }
  }, [editRoute, isNew, selectedIndex, routeEntries, loadFiles, reload]);

  const handleDelete = useCallback(async () => {
    if (selectedIndex === null) return;
    const entry = routeEntries[selectedIndex];
    if (!confirm(`Delete route "${entry.route.id}"? This will delete the file.`)) return;
    try {
      await api.deleteFile(entry.filePath);
      showToast({ type: "success", message: `Route "${entry.route.id}" deleted` });
      setSelectedIndex(null);
      setEditRoute(null);
      await loadFiles();
      await reload();
    } catch (err) {
      showToast({ type: "error", message: `Failed to delete route: ${err}` });
    }
  }, [selectedIndex, routeEntries, loadFiles, reload]);

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
          <h2 className="text-sm font-semibold text-gray-800">
            Routes ({routeEntries.length})
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
          {routeEntries.map((entry, index) => (
            <button
              key={`${entry.filePath}-${entry.route.id}`}
              onClick={() => selectRoute(index)}
              className={`w-full text-left px-4 py-2.5 hover:bg-gray-50 flex items-center gap-3 ${
                selectedIndex === index && !isNew ? "bg-blue-50" : ""
              }`}
            >
              <span
                className={`px-2 py-0.5 text-xs font-mono font-medium rounded ${
                  methodColors[entry.route.method] ?? "bg-gray-100 text-gray-700"
                }`}
              >
                {entry.route.method}
              </span>
              <div className="flex-1 min-w-0">
                <div className="text-sm font-medium text-gray-800 truncate">
                  {entry.route.path}
                </div>
                {entry.route.summary && (
                  <div className="text-xs text-gray-400 truncate">
                    {entry.route.summary}
                  </div>
                )}
              </div>
              {entry.route.trigger?.workflow && (
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    goToWorkflow(entry.route.trigger!.workflow);
                  }}
                  className="text-blue-400 hover:text-blue-600 shrink-0"
                  title="Open workflow"
                >
                  <ExternalLink size={12} />
                </button>
              )}
            </button>
          ))}
          {routeEntries.length === 0 && (
            <div className="p-4 text-sm text-gray-400">
              No routes configured. Click "New" to create one.
            </div>
          )}
        </div>
      </div>

      {/* Route editor */}
      <div className="flex-1 overflow-y-auto p-6">
        {editRoute ? (
          <RouteFormPanel
            route={editRoute}
            workflows={files?.workflows ?? []}
            onChange={setEditRoute}
            onSave={handleSave}
            onDelete={!isNew ? handleDelete : undefined}
            saving={saving}
            isNew={isNew}
          />
        ) : (
          <div className="text-sm text-gray-400">
            Select a route to edit or click "New" to create one.
          </div>
        )}
      </div>
    </div>
  );
}
