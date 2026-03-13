import { useEffect, useState, useCallback, useMemo } from "react";
import { ExternalLink, Plus, ChevronRight, ChevronDown, Copy, Shield, Download, FileJson } from "lucide-react";
import { ViewHeader } from "@/components/layout/ViewHeader";
import Editor from "@monaco-editor/react";
import * as api from "@/api/client";
import { useEditorStore } from "@/stores/editor";
import { RouteFormPanel, type RouteConfig } from "./RouteFormPanel";
import { RouteGroupFormPanel } from "./RouteGroupFormPanel";
import { TryItPanel } from "./TryItPanel";
import { showToast } from "@/components/panels/Toast";
import type { SchemaInfo, RouteGroupConfig } from "@/types";

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

type TabType = "editor" | "tryit" | "json" | "openapi";

// --- Path tree types ---
interface TreeNode {
  segment: string;
  fullPath: string;
  children: Map<string, TreeNode>;
  routes: RouteFileEntry[];
}

function buildTree(entries: RouteFileEntry[]): TreeNode {
  const root: TreeNode = { segment: "", fullPath: "", children: new Map(), routes: [] };

  for (const entry of entries) {
    const parts = entry.route.path.split("/").filter(Boolean);
    let node = root;
    let path = "";
    for (const part of parts) {
      path += "/" + part;
      if (!node.children.has(part)) {
        node.children.set(part, {
          segment: part,
          fullPath: path,
          children: new Map(),
          routes: [],
        });
      }
      node = node.children.get(part)!;
    }
    node.routes.push(entry);
  }

  return root;
}

// Collapse single-child interior nodes
function collapseTree(node: TreeNode): TreeNode {
  // Recursively collapse children first
  const collapsedChildren = new Map<string, TreeNode>();
  for (const [key, child] of node.children) {
    collapsedChildren.set(key, collapseTree(child));
  }
  node.children = collapsedChildren;

  // If this node has exactly one child and no routes, merge with child
  if (node.children.size === 1 && node.routes.length === 0) {
    const [, child] = [...node.children.entries()][0];
    return {
      segment: node.segment ? node.segment + "/" + child.segment : child.segment,
      fullPath: child.fullPath,
      children: child.children,
      routes: child.routes,
    };
  }

  return node;
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
  const [activeTab, setActiveTab] = useState<TabType>("editor");

  // Middleware + schema data
  const [middlewareNames, setMiddlewareNames] = useState<string[]>([]);
  const [middlewarePresets, setMiddlewarePresets] = useState<Record<string, string[]>>({});
  const [schemas, setSchemas] = useState<SchemaInfo[]>([]);

  // Root config (for route groups)
  const [rootConfig, setRootConfig] = useState<Record<string, unknown>>({});
  const [rootPath, setRootPath] = useState("");

  // Route group selection
  const [selectedGroup, setSelectedGroup] = useState<string | null>(null);
  const [editGroup, setEditGroup] = useState<RouteGroupConfig | null>(null);
  const [savingGroup, setSavingGroup] = useState(false);

  // OpenAPI state
  const [openApiSpec, setOpenApiSpec] = useState<string | null>(null);
  const [openApiLoading, setOpenApiLoading] = useState(false);

  // Tree expansion state
  const [expanded, setExpanded] = useState<Set<string>>(new Set());

  const tree = useMemo(() => {
    const raw = buildTree(routeEntries);
    const collapsed = collapseTree(raw);
    // Collect all group paths for default-expanded
    const paths = new Set<string>();
    const walk = (n: TreeNode) => {
      if (n.children.size > 0) {
        paths.add(n.fullPath);
        for (const child of n.children.values()) walk(child);
      }
    };
    walk(collapsed);
    // Only set expanded on initial load
    if (routeEntries.length > 0 && expanded.size === 0) {
      setExpanded(paths);
    }
    return collapsed;
  }, [routeEntries]); // eslint-disable-line react-hooks/exhaustive-deps

  const reload = useCallback(async () => {
    if (!files?.routes) return;
    setLoading(true);
    try {
      const [entries, mwInfo, schemaList, rootData] = await Promise.all([
        (async () => {
          const result: RouteFileEntry[] = [];
          await Promise.all(
            files.routes.map(async (path) => {
              const data = await api.readFile(path);
              const routes = Array.isArray(data)
                ? (data as RouteConfig[])
                : [data as RouteConfig];
              for (const route of routes) {
                result.push({ filePath: path, route });
              }
            })
          );
          return result;
        })(),
        api.listMiddleware(),
        api.listSchemas(),
        files.root
          ? (api.readFile(files.root) as Promise<Record<string, unknown>>)
          : Promise.resolve({}),
      ]);
      setRouteEntries(entries);
      const instanceNames = Object.keys(mwInfo.instances ?? {});
      setMiddlewareNames([...mwInfo.middleware.map((m) => m.name), ...instanceNames]);
      setMiddlewarePresets(mwInfo.presets);
      setSchemas(schemaList);
      setRootConfig(rootData);
      if (files.root) setRootPath(files.root);
    } finally {
      setLoading(false);
    }
  }, [files?.routes, files?.root]);

  useEffect(() => {
    reload();
  }, [reload]);

  const routeGroups = useMemo(
    () => (rootConfig?.route_groups ?? {}) as Record<string, RouteGroupConfig>,
    [rootConfig]
  );

  const selectGroup = useCallback(
    (fullPath: string) => {
      setSelectedGroup(fullPath);
      setSelectedIndex(null);
      setEditRoute(null);
      setIsNew(false);
      setEditGroup(routeGroups[fullPath] ? structuredClone(routeGroups[fullPath]) : {});
    },
    [routeGroups]
  );

  const selectRoute = useCallback(
    (index: number) => {
      setSelectedIndex(index);
      setEditRoute(structuredClone(routeEntries[index].route));
      setIsNew(false);
      setActiveTab("editor");
      setSelectedGroup(null);
      setEditGroup(null);
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
    setActiveTab("editor");
    setSelectedGroup(null);
    setEditGroup(null);
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
      if (!clean.middleware_preset) delete clean.middleware_preset;
      if (!clean.params?.schema) delete clean.params;
      if (!clean.query?.schema) delete clean.query;
      if (!clean.body || (!clean.body.schema && !clean.body.raw && clean.body.validate !== false && !clean.body.content_type))
        delete clean.body;
      else {
        if (clean.body.validate === true || clean.body.validate === undefined)
          delete clean.body.validate;
        if (!clean.body.content_type) delete clean.body.content_type;
      }
      if (clean.trigger) {
        if (!clean.trigger.workflow) delete (clean as Record<string, unknown>).trigger;
        else {
          if (!clean.trigger.input || Object.keys(clean.trigger.input).length === 0)
            delete clean.trigger.input;
          if (!clean.trigger.files?.length) delete clean.trigger.files;
          if (!clean.trigger.raw_body) delete clean.trigger.raw_body;
        }
      }
      // Clean response
      if (clean.response) {
        if (clean.response.validate === "off" || !clean.response.validate)
          delete clean.response.validate;
        if (clean.response.statuses) {
          // Remove entries with no schema
          for (const [code, entry] of Object.entries(clean.response.statuses)) {
            if (!entry.schema && !entry.description) {
              delete clean.response.statuses[code];
            }
          }
          if (Object.keys(clean.response.statuses).length === 0)
            delete clean.response.statuses;
        }
        if (!clean.response.validate && !clean.response.statuses)
          delete clean.response;
      }

      if (isNew) {
        const filePath = `routes/${clean.id}.json`;
        await api.writeFile(filePath, clean);
        showToast({ type: "success", message: `Route "${clean.id}" created` });
      } else if (selectedIndex !== null) {
        const entry = routeEntries[selectedIndex];
        await api.writeFile(entry.filePath, clean);
        showToast({ type: "success", message: `Route "${clean.id}" saved` });
      }

      await loadFiles();
      await reload();
      setIsNew(false);
      const newEntries = routeEntries;
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

  const saveGroup = useCallback(async () => {
    if (selectedGroup === null || !editGroup || !rootPath) return;
    setSavingGroup(true);
    try {
      const updated = structuredClone(rootConfig);
      const groups = (updated.route_groups ?? {}) as Record<string, RouteGroupConfig>;

      // Clean empty fields
      const clean = { ...editGroup };
      if (!clean.middleware_preset) delete clean.middleware_preset;
      if (!clean.middleware?.length) delete clean.middleware;
      if (!clean.tags?.length) delete clean.tags;

      if (Object.keys(clean).length > 0) {
        groups[selectedGroup] = clean;
      } else {
        groups[selectedGroup] = {};
      }
      updated.route_groups = groups;

      await api.writeFile(rootPath, updated);
      showToast({ type: "success", message: `Group "${selectedGroup}" saved` });
      await reload();
    } catch (err) {
      showToast({ type: "error", message: `Failed to save group: ${err}` });
    } finally {
      setSavingGroup(false);
    }
  }, [selectedGroup, editGroup, rootPath, rootConfig, reload]);

  const deleteGroup = useCallback(async () => {
    if (selectedGroup === null || !rootPath) return;
    if (!confirm(`Delete group "${selectedGroup}"?`)) return;
    try {
      const updated = structuredClone(rootConfig);
      const groups = (updated.route_groups ?? {}) as Record<string, RouteGroupConfig>;
      delete groups[selectedGroup];
      if (Object.keys(groups).length > 0) {
        updated.route_groups = groups;
      } else {
        delete updated.route_groups;
      }
      await api.writeFile(rootPath, updated);
      showToast({ type: "success", message: "Group deleted" });
      setSelectedGroup(null);
      setEditGroup(null);
      await reload();
    } catch (err) {
      showToast({ type: "error", message: `Failed to delete group: ${err}` });
    }
  }, [selectedGroup, rootPath, rootConfig, reload]);

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

  const toggleExpand = useCallback((path: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  }, []);

  if (loading) {
    return <div className="p-6 text-sm text-gray-400">Loading routes...</div>;
  }

  return (
    <div className="flex-1 flex flex-col min-h-0">
      <ViewHeader title="Routes" subtitle="HTTP route definitions — map URL patterns to workflows" />
      <div className="flex-1 flex min-h-0">
      {/* Route list */}
      <div className="w-96 border-r border-gray-200 overflow-y-auto">
        <div className="px-4 py-3 border-b border-gray-200 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-gray-800">
            Routes ({routeEntries.length})
          </h2>
          <div className="flex items-center gap-1">
            <button
              onClick={async () => {
                setSelectedIndex(null);
                setEditRoute(null);
                setSelectedGroup(null);
                setEditGroup(null);
                setActiveTab("openapi");
                setOpenApiLoading(true);
                try {
                  const spec = await api.getOpenAPISpec();
                  setOpenApiSpec(JSON.stringify(spec, null, 2));
                } catch (err) {
                  showToast({ type: "error", message: `Failed to load OpenAPI spec: ${err}` });
                } finally {
                  setOpenApiLoading(false);
                }
              }}
              className={`flex items-center gap-1 px-2 py-1 text-xs rounded ${
                activeTab === "openapi"
                  ? "text-blue-700 bg-blue-50"
                  : "text-gray-500 hover:bg-gray-100"
              }`}
              title="OpenAPI Spec"
            >
              <FileJson size={14} />
              OpenAPI
            </button>
            <button
              onClick={startNew}
              className="flex items-center gap-1 px-2 py-1 text-xs text-blue-600 hover:bg-blue-50 rounded"
            >
              <Plus size={14} />
              New
            </button>
          </div>
        </div>
        <div>
          {tree.children.size > 0 ? (
            [...tree.children.values()].map((child) => (
              <TreeNodeView
                key={child.fullPath}
                node={child}
                depth={0}
                expanded={expanded}
                toggleExpand={toggleExpand}
                selectedIndex={selectedIndex}
                isNew={isNew}
                routeEntries={routeEntries}
                selectRoute={selectRoute}
                goToWorkflow={goToWorkflow}
                methodColors={methodColors}
                routeGroups={routeGroups}
                selectedGroup={selectedGroup}
                onSelectGroup={selectGroup}
              />
            ))
          ) : (
            // Flat list fallback for routes at root
            tree.routes.map((entry) => {
              const idx = routeEntries.indexOf(entry);
              return (
                <RouteItem
                  key={`${entry.filePath}-${entry.route.id}`}
                  entry={entry}
                  index={idx}
                  selected={selectedIndex === idx && !isNew}
                  onSelect={selectRoute}
                  goToWorkflow={goToWorkflow}
                  methodColors={methodColors}
                />
              );
            })
          )}
          {routeEntries.length === 0 && (
            <div className="p-4 text-sm text-gray-400">
              No routes configured. Click "New" to create one.
            </div>
          )}
        </div>
      </div>

      {/* Route editor / Group editor / OpenAPI */}
      <div className="flex-1 flex flex-col min-h-0">
        {activeTab === "openapi" ? (
          <div className="flex-1 overflow-y-auto p-6">
            <div className="max-w-4xl space-y-3">
              <div className="flex items-center justify-between">
                <h3 className="text-lg font-semibold text-gray-900">OpenAPI Specification</h3>
                {openApiSpec && (
                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => {
                        navigator.clipboard.writeText(openApiSpec);
                        showToast({ type: "success", message: "Copied to clipboard" });
                      }}
                      className="flex items-center gap-1 px-2 py-1 text-xs text-gray-500 hover:text-gray-700 hover:bg-gray-100 rounded"
                    >
                      <Copy size={12} />
                      Copy
                    </button>
                    <button
                      onClick={() => {
                        const blob = new Blob([openApiSpec], { type: "application/json" });
                        const url = URL.createObjectURL(blob);
                        const a = document.createElement("a");
                        a.href = url;
                        a.download = "openapi.json";
                        a.click();
                        URL.revokeObjectURL(url);
                      }}
                      className="flex items-center gap-1 px-2 py-1 text-xs text-blue-500 hover:text-blue-700 hover:bg-blue-50 rounded"
                    >
                      <Download size={12} />
                      Download
                    </button>
                  </div>
                )}
              </div>
              {openApiLoading ? (
                <div className="text-sm text-gray-400">Generating OpenAPI spec...</div>
              ) : openApiSpec ? (
                <div className="border border-gray-200 rounded overflow-hidden">
                  <Editor
                    height="600px"
                    language="json"
                    value={openApiSpec}
                    options={{
                      minimap: { enabled: false },
                      fontSize: 13,
                      scrollBeyondLastLine: false,
                      lineNumbers: "on",
                      readOnly: true,
                      wordWrap: "on",
                    }}
                  />
                </div>
              ) : (
                <div className="text-sm text-gray-400">Failed to load OpenAPI spec.</div>
              )}
            </div>
          </div>
        ) : selectedGroup !== null && editGroup ? (
          <div className="flex-1 overflow-y-auto p-6">
            <RouteGroupFormPanel
              prefix={selectedGroup}
              group={editGroup}
              middlewareNames={middlewareNames}
              middlewarePresets={middlewarePresets}
              onChange={setEditGroup}
              onSave={saveGroup}
              onDelete={routeGroups[selectedGroup] ? deleteGroup : undefined}
              saving={savingGroup}
              isNew={!routeGroups[selectedGroup]}
            />
          </div>
        ) : editRoute ? (
          <>
            {/* Tab bar */}
            <div className="flex items-center border-b border-gray-200 bg-gray-50 shrink-0">
              {(["editor", "tryit", "json"] as TabType[]).map((tab) => {
                const labels: Record<string, string> = {
                  editor: "Editor",
                  tryit: "Try It",
                  json: "JSON",
                };
                // Disable Try It for new routes
                const disabled = tab === "tryit" && isNew;
                return (
                  <button
                    key={tab}
                    onClick={() => !disabled && setActiveTab(tab)}
                    disabled={disabled}
                    className={`px-4 py-2 text-sm transition-colors ${
                      activeTab === tab
                        ? "bg-white text-blue-700 font-medium border-b-2 border-b-blue-500"
                        : disabled
                          ? "text-gray-300 cursor-not-allowed"
                          : "text-gray-600 hover:bg-gray-100"
                    }`}
                  >
                    {labels[tab]}
                  </button>
                );
              })}
            </div>

            {/* Tab content */}
            <div className="flex-1 overflow-y-auto p-6">
              <div className="max-w-3xl">
                {activeTab === "editor" && (
                  <RouteFormPanel
                    route={editRoute}
                    workflows={files?.workflows ?? []}
                    middlewareNames={middlewareNames}
                    middlewarePresets={middlewarePresets}
                    schemas={schemas}
                    onChange={setEditRoute}
                    onSave={handleSave}
                    onDelete={!isNew ? handleDelete : undefined}
                    saving={saving}
                    isNew={isNew}
                  />
                )}
                {activeTab === "tryit" && !isNew && (
                  <TryItPanel route={editRoute} />
                )}
                {activeTab === "json" && (
                  <div className="space-y-2">
                    <div className="flex items-center justify-between">
                      <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider">
                        JSON Preview
                      </h4>
                      <button
                        onClick={() => {
                          navigator.clipboard.writeText(JSON.stringify(editRoute, null, 2));
                          showToast({ type: "success", message: "Copied to clipboard" });
                        }}
                        className="flex items-center gap-1 px-2 py-1 text-xs text-gray-500 hover:text-gray-700 hover:bg-gray-100 rounded"
                      >
                        <Copy size={12} />
                        Copy
                      </button>
                    </div>
                    <div className="border border-gray-200 rounded overflow-hidden">
                      <Editor
                        height="500px"
                        language="json"
                        value={JSON.stringify(editRoute, null, 2)}
                        options={{
                          minimap: { enabled: false },
                          fontSize: 13,
                          scrollBeyondLastLine: false,
                          lineNumbers: "on",
                          readOnly: true,
                          wordWrap: "on",
                        }}
                      />
                    </div>
                  </div>
                )}
              </div>
            </div>
          </>
        ) : (
          <div className="flex-1 flex items-center justify-center text-sm text-gray-400">
            Select a route to edit or click "New" to create one.
          </div>
        )}
      </div>
      </div>
    </div>
  );
}

// --- Tree rendering components ---

function TreeNodeView({
  node,
  depth,
  expanded,
  toggleExpand,
  selectedIndex,
  isNew,
  routeEntries,
  selectRoute,
  goToWorkflow,
  methodColors,
  routeGroups,
  selectedGroup,
  onSelectGroup,
}: {
  node: TreeNode;
  depth: number;
  expanded: Set<string>;
  toggleExpand: (path: string) => void;
  selectedIndex: number | null;
  isNew: boolean;
  routeEntries: RouteFileEntry[];
  selectRoute: (index: number) => void;
  goToWorkflow: (workflowId: string) => void;
  methodColors: Record<string, string>;
  routeGroups: Record<string, RouteGroupConfig>;
  selectedGroup: string | null;
  onSelectGroup: (fullPath: string) => void;
}) {
  const hasChildren = node.children.size > 0 || node.routes.length > 0;
  const isExpanded = expanded.has(node.fullPath);
  const isGroup = node.children.size > 0 || node.routes.length > 1;

  if (!isGroup && node.routes.length === 1) {
    // Leaf: single route
    const entry = node.routes[0];
    const idx = routeEntries.indexOf(entry);
    return (
      <RouteItem
        key={`${entry.filePath}-${entry.route.id}`}
        entry={entry}
        index={idx}
        selected={selectedIndex === idx && !isNew}
        onSelect={selectRoute}
        goToWorkflow={goToWorkflow}
        methodColors={methodColors}
        indent={depth}
      />
    );
  }

  const hasGroup = !!routeGroups[node.fullPath];
  const isGroupSelected = selectedGroup === node.fullPath;

  return (
    <div>
      {/* Group header */}
      <div
        className={`w-full flex items-center gap-1.5 px-4 py-1.5 text-xs font-medium text-gray-500 hover:bg-gray-50 ${
          isGroupSelected ? "bg-blue-50" : ""
        }`}
        style={{ paddingLeft: `${16 + depth * 12}px` }}
      >
        <button
          onClick={() => toggleExpand(node.fullPath)}
          className="shrink-0"
        >
          {hasChildren ? (
            isExpanded ? <ChevronDown size={12} /> : <ChevronRight size={12} />
          ) : null}
        </button>
        <button
          onClick={() => onSelectGroup(node.fullPath)}
          className="flex items-center gap-1.5 min-w-0"
        >
          <span className="font-mono text-gray-600">/{node.segment}</span>
          {hasGroup && <Shield size={12} className="text-blue-500 shrink-0" />}
          <span className="text-gray-400 ml-1">
            ({countRoutes(node)})
          </span>
        </button>
      </div>

      {/* Children */}
      {isExpanded && (
        <>
          {node.routes.map((entry) => {
            const idx = routeEntries.indexOf(entry);
            return (
              <RouteItem
                key={`${entry.filePath}-${entry.route.id}`}
                entry={entry}
                index={idx}
                selected={selectedIndex === idx && !isNew}
                onSelect={selectRoute}
                goToWorkflow={goToWorkflow}
                methodColors={methodColors}
                indent={depth + 1}
              />
            );
          })}
          {[...node.children.values()].map((child) => (
            <TreeNodeView
              key={child.fullPath}
              node={child}
              depth={depth + 1}
              expanded={expanded}
              toggleExpand={toggleExpand}
              selectedIndex={selectedIndex}
              isNew={isNew}
              routeEntries={routeEntries}
              selectRoute={selectRoute}
              goToWorkflow={goToWorkflow}
              methodColors={methodColors}
              routeGroups={routeGroups}
              selectedGroup={selectedGroup}
              onSelectGroup={onSelectGroup}
            />
          ))}
        </>
      )}
    </div>
  );
}

function RouteItem({
  entry,
  index,
  selected,
  onSelect,
  goToWorkflow,
  methodColors,
  indent = 0,
}: {
  entry: RouteFileEntry;
  index: number;
  selected: boolean;
  onSelect: (index: number) => void;
  goToWorkflow: (workflowId: string) => void;
  methodColors: Record<string, string>;
  indent?: number;
}) {
  return (
    <button
      onClick={() => onSelect(index)}
      className={`w-full text-left px-4 py-2.5 hover:bg-gray-50 flex items-center gap-3 border-b border-gray-100 ${
        selected ? "bg-blue-50" : ""
      }`}
      style={{ paddingLeft: `${16 + indent * 12}px` }}
    >
      <span
        className={`px-2 py-0.5 text-xs font-mono font-medium rounded shrink-0 ${
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
  );
}

function countRoutes(node: TreeNode): number {
  let count = node.routes.length;
  for (const child of node.children.values()) {
    count += countRoutes(child);
  }
  return count;
}
