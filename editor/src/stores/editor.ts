import { create } from "zustand";
import type {
  ViewType,
  FileList,
  WorkflowConfig,
  WorkflowNode,
  WorkflowEdge,
  ValidationError,
  NodeDescriptor,
  VarInfo,
} from "@/types";
import * as api from "@/api/client";
import * as history from "@/stores/history";
import { showToast } from "@/components/panels/Toast";
import { autoLayout } from "@/components/canvas/autoLayout";

type SaveStatus = "idle" | "saving" | "saved" | "error";

interface EditorState {
  // Navigation
  activeView: ViewType;
  setActiveView: (view: ViewType) => void;

  // Files
  files: FileList | null;
  loadFiles: () => Promise<void>;

  // Shared variables
  vars: VarInfo[];
  loadVars: () => Promise<void>;

  // Workflows
  activeWorkflowPath: string | null;
  activeWorkflow: WorkflowConfig | null;
  _rawWorkflow: Record<string, unknown> | null; // original JSON for round-trip
  openTabs: string[]; // ordered list of open workflow paths
  setActiveWorkflow: (path: string | null) => void;
  closeTab: (path: string) => void;
  loadWorkflow: (path: string) => Promise<void>;

  // Workflow mutations
  updateNodeConfig: (nodeId: string, config: Record<string, unknown>) => void;
  updateNodeServices: (nodeId: string, services: Record<string, string>) => void;
  renameNode: (oldId: string, newId: string) => void;
  updateNodeAlias: (nodeId: string, alias: string | undefined) => void;
  addNode: (node: WorkflowNode) => void;
  removeNode: (nodeId: string) => void;
  updateNodePosition: (nodeId: string, position: { x: number; y: number }) => void;
  addEdge: (edge: WorkflowEdge) => void;
  removeEdge: (from: string, output: string, to: string) => void;
  updateEdgeRetry: (index: number, retry: WorkflowEdge["retry"]) => void;
  setWorkflow: (wf: WorkflowConfig) => void;
  updateWorkflowMeta: (patch: { description?: string; version?: string }) => void;

  // History
  undo: () => void;
  redo: () => void;

  // Selection
  selectedNodeId: string | null;
  selectedNodeIds: Set<string>;
  selectedEdgeIndex: number | null;
  selectNode: (id: string | null) => void;
  setSelectedNodeIds: (ids: Set<string>) => void;
  selectEdge: (index: number | null) => void;
  deselectAll: () => void;
  removeSelectedNodes: () => void;

  // Node registry
  nodeTypes: NodeDescriptor[];
  loadNodeTypes: () => Promise<void>;

  // Save
  saveStatus: SaveStatus;
  saveWorkflow: () => Promise<void>;
  _saveTimer: ReturnType<typeof setTimeout> | null;
  _debounceSave: () => void;
  autoSave: boolean;
  setAutoSave: (enabled: boolean) => void;
  validateAndSave: () => Promise<void>;

  // Validation
  validationErrors: ValidationError[];
  setValidationErrors: (errors: ValidationError[]) => void;

  // Dirty state
  dirtyFiles: Set<string>;
  markDirty: (path: string) => void;
  markClean: (path: string) => void;

  // Doc navigation
  pendingDocPath: string | null;
  openDoc: (docPath: string) => void;
  clearPendingDocPath: () => void;
}

/**
 * Convert raw Noda workflow JSON (nodes as object map) to the editor's
 * internal format (nodes as array). Also handles the case where nodes
 * is already an array (e.g. after a round-trip through the editor).
 */
function normalizeWorkflow(raw: Record<string, unknown>): WorkflowConfig {
  const rawNodes = raw.nodes;
  let nodes: WorkflowNode[];

  if (Array.isArray(rawNodes)) {
    nodes = rawNodes as WorkflowNode[];
  } else if (rawNodes && typeof rawNodes === "object") {
    // Convert { nodeId: { type, config, ... } } → [{ id, type, config, ... }]
    nodes = Object.entries(rawNodes as Record<string, Record<string, unknown>>).map(
      ([id, node]) => ({
        id,
        type: node.type as string,
        config: node.config as Record<string, unknown> | undefined,
        as: node.as as string | undefined,
        services: node.services as Record<string, string> | undefined,
        position: node.position as { x: number; y: number } | undefined,
      })
    );
  } else {
    nodes = [];
  }

  const rawEdges = raw.edges;
  let edges: WorkflowEdge[];

  if (Array.isArray(rawEdges)) {
    edges = (rawEdges as Record<string, unknown>[]).map((e) => ({
      from: e.from as string,
      output: (e.output as string) ?? "success",
      to: e.to as string,
      retry: e.retry as WorkflowEdge["retry"],
    }));
  } else {
    edges = [];
  }

  return {
    description: raw.description as string | undefined,
    version: raw.version as string | undefined,
    nodes,
    edges,
  };
}

/**
 * Convert the editor's internal format back to the Noda JSON format
 * (nodes as object map) for saving.
 */
function denormalizeWorkflow(
  wf: WorkflowConfig,
  original: Record<string, unknown>
): Record<string, unknown> {
  const nodesMap: Record<string, Record<string, unknown>> = {};
  for (const node of wf.nodes) {
    const entry: Record<string, unknown> = { type: node.type };
    if (node.services && Object.keys(node.services).length > 0)
      entry.services = node.services;
    if (node.config && Object.keys(node.config).length > 0)
      entry.config = node.config;
    if (node.as) entry.as = node.as;
    // Don't persist position into config (it's editor-only state)
    nodesMap[node.id] = entry;
  }

  const edges = wf.edges.map((e) => {
    const edge: Record<string, unknown> = { from: e.from, to: e.to };
    if (e.output && e.output !== "success") edge.output = e.output;
    if (e.retry) edge.retry = e.retry;
    return edge;
  });

  // Preserve top-level fields from the original file (id, name, etc.)
  const result: Record<string, unknown> = { ...original, nodes: nodesMap, edges };
  if (wf.description) result.description = wf.description;
  else delete result.description;
  if (wf.version) result.version = wf.version;
  else delete result.version;
  return result;
}

export const useEditorStore = create<EditorState>((set, get) => ({
  // Navigation
  activeView: "workflows",
  setActiveView: (view) => set({ activeView: view }),

  // Files
  files: null,
  loadFiles: async () => {
    const files = await api.listFiles();
    set({ files });
  },

  // Shared variables
  vars: [],
  loadVars: async () => {
    try {
      const vars = await api.listVars();
      set({ vars });
    } catch {
      // vars.json may not exist
    }
  },

  // Workflows
  activeWorkflowPath: null,
  activeWorkflow: null,
  _rawWorkflow: null,
  openTabs: [],
  setActiveWorkflow: (path) => {
    if (path === null) {
      set({ activeWorkflowPath: null, activeWorkflow: null, _rawWorkflow: null, selectedNodeId: null, selectedNodeIds: new Set(), selectedEdgeIndex: null });
    } else {
      // Add to tabs if not already open
      const tabs = get().openTabs;
      if (!tabs.includes(path)) {
        set({ openTabs: [...tabs, path] });
      }
      get().loadWorkflow(path);
    }
  },
  closeTab: (path) => {
    const { openTabs, activeWorkflowPath } = get();
    const newTabs = openTabs.filter((t) => t !== path);
    set({ openTabs: newTabs });
    // If closing the active tab, switch to the last remaining tab (or none)
    if (activeWorkflowPath === path) {
      if (newTabs.length > 0) {
        get().loadWorkflow(newTabs[newTabs.length - 1]);
      } else {
        set({ activeWorkflowPath: null, activeWorkflow: null, _rawWorkflow: null, selectedNodeId: null, selectedNodeIds: new Set(), selectedEdgeIndex: null });
      }
    }
  },
  loadWorkflow: async (path) => {
    const raw = (await api.readFile(path)) as Record<string, unknown>;
    let data = normalizeWorkflow(raw);

    // Auto-layout on first load when no nodes have saved positions
    const needsLayout = data.nodes.length > 0 && data.nodes.every((n) => !n.position);
    if (needsLayout) {
      data = await autoLayout(data);
    }

    set({
      activeWorkflowPath: path,
      activeWorkflow: data,
      _rawWorkflow: raw,
      selectedNodeId: null,
      selectedNodeIds: new Set(),
      selectedEdgeIndex: null,
    });
  },

  // Workflow mutations (all push history before mutating)
  updateNodeConfig: (nodeId, config) => {
    const { activeWorkflow, activeWorkflowPath } = get();
    if (!activeWorkflow || !activeWorkflowPath) return;
    history.pushSnapshot(activeWorkflowPath, activeWorkflow);
    const nodes = activeWorkflow.nodes.map((n) =>
      n.id === nodeId ? { ...n, config } : n
    );
    set({ activeWorkflow: { ...activeWorkflow, nodes } });
    get()._debounceSave();
  },

  renameNode: (oldId, newId) => {
    const { activeWorkflow, activeWorkflowPath } = get();
    if (!activeWorkflow || !activeWorkflowPath || !newId || oldId === newId) return;
    // Check for duplicate
    if (activeWorkflow.nodes.some((n) => n.id === newId)) return;
    history.pushSnapshot(activeWorkflowPath, activeWorkflow);
    const nodes = activeWorkflow.nodes.map((n) =>
      n.id === oldId ? { ...n, id: newId } : n
    );
    const edges = activeWorkflow.edges.map((e) => ({
      ...e,
      from: e.from === oldId ? newId : e.from,
      to: e.to === oldId ? newId : e.to,
    }));
    set({
      activeWorkflow: { ...activeWorkflow, nodes, edges },
      selectedNodeId: newId,
    });
    get()._debounceSave();
  },

  updateNodeAlias: (nodeId, alias) => {
    const { activeWorkflow, activeWorkflowPath } = get();
    if (!activeWorkflow || !activeWorkflowPath) return;
    history.pushSnapshot(activeWorkflowPath, activeWorkflow);
    const nodes = activeWorkflow.nodes.map((n) =>
      n.id === nodeId ? { ...n, as: alias } : n
    );
    set({ activeWorkflow: { ...activeWorkflow, nodes } });
    get()._debounceSave();
  },

  updateNodeServices: (nodeId, services) => {
    const { activeWorkflow, activeWorkflowPath } = get();
    if (!activeWorkflow || !activeWorkflowPath) return;
    history.pushSnapshot(activeWorkflowPath, activeWorkflow);
    const nodes = activeWorkflow.nodes.map((n) =>
      n.id === nodeId ? { ...n, services } : n
    );
    set({ activeWorkflow: { ...activeWorkflow, nodes } });
    get()._debounceSave();
  },

  addNode: (node) => {
    const { activeWorkflow, activeWorkflowPath } = get();
    if (!activeWorkflow || !activeWorkflowPath) return;
    history.pushSnapshot(activeWorkflowPath, activeWorkflow);
    const nodes = [...activeWorkflow.nodes, node];
    set({ activeWorkflow: { ...activeWorkflow, nodes } });
    get()._debounceSave();
  },

  removeNode: (nodeId) => {
    const { activeWorkflow, activeWorkflowPath, selectedNodeId } = get();
    if (!activeWorkflow || !activeWorkflowPath) return;
    history.pushSnapshot(activeWorkflowPath, activeWorkflow);
    const nodes = activeWorkflow.nodes.filter((n) => n.id !== nodeId);
    const edges = activeWorkflow.edges.filter(
      (e) => e.from !== nodeId && e.to !== nodeId
    );
    const newSelectedIds = new Set(get().selectedNodeIds);
    newSelectedIds.delete(nodeId);
    set({
      activeWorkflow: { ...activeWorkflow, nodes, edges },
      selectedNodeId: selectedNodeId === nodeId ? null : selectedNodeId,
      selectedNodeIds: newSelectedIds,
    });
    get()._debounceSave();
  },

  updateNodePosition: (nodeId, position) =>
    set((state) => {
      if (!state.activeWorkflow) return state;
      const nodes = state.activeWorkflow.nodes.map((n) =>
        n.id === nodeId ? { ...n, position } : n
      );
      return { activeWorkflow: { ...state.activeWorkflow, nodes } };
    }),

  addEdge: (edge) => {
    const { activeWorkflow, activeWorkflowPath } = get();
    if (!activeWorkflow || !activeWorkflowPath) return;
    const exists = activeWorkflow.edges.some(
      (e) => e.from === edge.from && e.output === edge.output && e.to === edge.to
    );
    if (exists) return;
    history.pushSnapshot(activeWorkflowPath, activeWorkflow);
    const edges = [...activeWorkflow.edges, edge];
    set({ activeWorkflow: { ...activeWorkflow, edges } });
    get()._debounceSave();
  },

  removeEdge: (from, output, to) => {
    const { activeWorkflow, activeWorkflowPath } = get();
    if (!activeWorkflow || !activeWorkflowPath) return;
    history.pushSnapshot(activeWorkflowPath, activeWorkflow);
    const edges = activeWorkflow.edges.filter(
      (e) => !(e.from === from && e.output === output && e.to === to)
    );
    set({ activeWorkflow: { ...activeWorkflow, edges } });
    get()._debounceSave();
  },

  updateEdgeRetry: (index, retry) => {
    const { activeWorkflow, activeWorkflowPath } = get();
    if (!activeWorkflow || !activeWorkflowPath) return;
    history.pushSnapshot(activeWorkflowPath, activeWorkflow);
    const edges = activeWorkflow.edges.map((e, i) =>
      i === index ? { ...e, retry } : e
    );
    set({ activeWorkflow: { ...activeWorkflow, edges } });
    get()._debounceSave();
  },

  setWorkflow: (wf) => {
    const { activeWorkflow, activeWorkflowPath } = get();
    if (!activeWorkflowPath) return;
    if (activeWorkflow) history.pushSnapshot(activeWorkflowPath, activeWorkflow);
    set({ activeWorkflow: wf });
    get()._debounceSave();
  },

  updateWorkflowMeta: (patch) => {
    const { activeWorkflow, activeWorkflowPath } = get();
    if (!activeWorkflow || !activeWorkflowPath) return;
    history.pushSnapshot(activeWorkflowPath, activeWorkflow);
    set({ activeWorkflow: { ...activeWorkflow, ...patch } });
    get()._debounceSave();
  },

  // History
  undo: () => {
    const { activeWorkflowPath, activeWorkflow } = get();
    if (!activeWorkflowPath || !activeWorkflow) return;
    const prev = history.undo(activeWorkflowPath, activeWorkflow);
    if (prev) {
      set({ activeWorkflow: prev });
      get()._debounceSave();
    }
  },

  redo: () => {
    const { activeWorkflowPath, activeWorkflow } = get();
    if (!activeWorkflowPath || !activeWorkflow) return;
    const next = history.redo(activeWorkflowPath, activeWorkflow);
    if (next) {
      set({ activeWorkflow: next });
      get()._debounceSave();
    }
  },

  // Selection
  selectedNodeId: null,
  selectedNodeIds: new Set(),
  selectedEdgeIndex: null,
  selectNode: (id) => set({
    selectedNodeId: id,
    selectedNodeIds: id ? new Set([id]) : new Set(),
    selectedEdgeIndex: null,
  }),
  setSelectedNodeIds: (ids) => {
    // When multiple nodes are selected, show config panel for the first one
    const arr = Array.from(ids);
    set({
      selectedNodeIds: ids,
      selectedNodeId: arr.length === 1 ? arr[0] : (arr.length > 0 ? arr[0] : null),
      selectedEdgeIndex: null,
    });
  },
  selectEdge: (index) => set({ selectedEdgeIndex: index, selectedNodeId: null, selectedNodeIds: new Set() }),
  deselectAll: () => set({ selectedNodeId: null, selectedNodeIds: new Set(), selectedEdgeIndex: null }),
  removeSelectedNodes: () => {
    const { activeWorkflow, activeWorkflowPath, selectedNodeIds } = get();
    if (!activeWorkflow || !activeWorkflowPath || selectedNodeIds.size === 0) return;
    history.pushSnapshot(activeWorkflowPath, activeWorkflow);
    const nodes = activeWorkflow.nodes.filter((n) => !selectedNodeIds.has(n.id));
    const edges = activeWorkflow.edges.filter(
      (e) => !selectedNodeIds.has(e.from) && !selectedNodeIds.has(e.to)
    );
    set({
      activeWorkflow: { ...activeWorkflow, nodes, edges },
      selectedNodeId: null,
      selectedNodeIds: new Set(),
    });
    get()._debounceSave();
  },

  // Node registry
  nodeTypes: [],
  loadNodeTypes: async () => {
    const nodeTypes = await api.listNodes();
    set({ nodeTypes });
  },

  // Save
  saveStatus: "idle" as SaveStatus,
  autoSave: false,
  setAutoSave: (enabled) => set({ autoSave: enabled }),
  _saveTimer: null,
  _debounceSave: () => {
    const state = get();
    if (state._saveTimer) clearTimeout(state._saveTimer);
    if (state.activeWorkflowPath) {
      get().markDirty(state.activeWorkflowPath);
    }
    if (!state.autoSave) return;
    const timer = setTimeout(() => {
      get().saveWorkflow();
    }, 300);
    set({ _saveTimer: timer });
  },
  validateAndSave: async () => {
    const { activeWorkflowPath, activeWorkflow, _rawWorkflow } = get();
    if (!activeWorkflowPath || !activeWorkflow) return;
    const payload = denormalizeWorkflow(activeWorkflow, _rawWorkflow ?? {});
    try {
      const result = await api.validateFile(activeWorkflowPath, payload);
      if (!result.valid) {
        set({ validationErrors: result.errors });
        showToast({
          type: "info",
          message: `Validation found ${result.errors.length} error${result.errors.length !== 1 ? "s" : ""}`,
          action: { label: "Save anyway", onClick: () => get().saveWorkflow() },
        });
        return;
      }
      set({ validationErrors: [] });
      await get().saveWorkflow();
    } catch {
      showToast({ type: "error", message: "Validation request failed" });
    }
  },
  saveWorkflow: async () => {
    const { activeWorkflowPath, activeWorkflow, _rawWorkflow } = get();
    if (!activeWorkflowPath || !activeWorkflow) return;
    const payload = denormalizeWorkflow(activeWorkflow, _rawWorkflow ?? {});
    // Skip save if only positions changed (positions are not persisted)
    if (JSON.stringify(payload) === JSON.stringify(_rawWorkflow)) {
      set({ saveStatus: "idle" });
      return;
    }
    set({ saveStatus: "saving" });
    try {
      await api.writeFile(activeWorkflowPath, payload);
      set({ saveStatus: "saved", _rawWorkflow: payload });
      get().markClean(activeWorkflowPath);
      setTimeout(() => {
        if (get().saveStatus === "saved") set({ saveStatus: "idle" });
      }, 2000);
    } catch {
      set({ saveStatus: "error" });
      showToast({
        type: "error",
        message: "Failed to save workflow",
        action: { label: "Retry", onClick: () => get().saveWorkflow() },
      });
    }
  },

  // Validation
  validationErrors: [],
  setValidationErrors: (errors) => set({ validationErrors: errors }),

  // Dirty state
  dirtyFiles: new Set(),
  markDirty: (path) =>
    set((state) => {
      const next = new Set(state.dirtyFiles);
      next.add(path);
      return { dirtyFiles: next };
    }),
  markClean: (path) =>
    set((state) => {
      const next = new Set(state.dirtyFiles);
      next.delete(path);
      return { dirtyFiles: next };
    }),

  // Doc navigation
  pendingDocPath: null,
  openDoc: (docPath) => set({ activeView: "docs", pendingDocPath: docPath }),
  clearPendingDocPath: () => set({ pendingDocPath: null }),
}));
