import { create } from "zustand";
import type {
  ViewType,
  FileList,
  WorkflowConfig,
  WorkflowNode,
  WorkflowEdge,
  ValidationError,
  NodeDescriptor,
} from "@/types";
import * as api from "@/api/client";
import * as history from "@/stores/history";
import { showToast } from "@/components/panels/Toast";

type SaveStatus = "idle" | "saving" | "saved" | "error";

interface EditorState {
  // Navigation
  activeView: ViewType;
  setActiveView: (view: ViewType) => void;

  // Files
  files: FileList | null;
  loadFiles: () => Promise<void>;

  // Workflows
  activeWorkflowPath: string | null;
  activeWorkflow: WorkflowConfig | null;
  _rawWorkflow: Record<string, unknown> | null; // original JSON for round-trip
  setActiveWorkflow: (path: string | null) => void;
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

  // History
  undo: () => void;
  redo: () => void;

  // Selection
  selectedNodeId: string | null;
  selectedEdgeIndex: number | null;
  selectNode: (id: string | null) => void;
  selectEdge: (index: number | null) => void;
  deselectAll: () => void;

  // Node registry
  nodeTypes: NodeDescriptor[];
  loadNodeTypes: () => Promise<void>;

  // Save
  saveStatus: SaveStatus;
  saveWorkflow: () => Promise<void>;
  _saveTimer: ReturnType<typeof setTimeout> | null;
  _debounceSave: () => void;

  // Validation
  validationErrors: ValidationError[];
  setValidationErrors: (errors: ValidationError[]) => void;

  // Dirty state
  dirtyFiles: Set<string>;
  markDirty: (path: string) => void;
  markClean: (path: string) => void;
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

  return { nodes, edges };
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
  return { ...original, nodes: nodesMap, edges };
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

  // Workflows
  activeWorkflowPath: null,
  activeWorkflow: null,
  _rawWorkflow: null,
  setActiveWorkflow: (path) => {
    if (path === null) {
      set({ activeWorkflowPath: null, activeWorkflow: null, _rawWorkflow: null, selectedNodeId: null, selectedEdgeIndex: null });
    } else {
      get().loadWorkflow(path);
    }
  },
  loadWorkflow: async (path) => {
    const raw = (await api.readFile(path)) as Record<string, unknown>;
    const data = normalizeWorkflow(raw);
    set({
      activeWorkflowPath: path,
      activeWorkflow: data,
      _rawWorkflow: raw,
      selectedNodeId: null,
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
    set({
      activeWorkflow: { ...activeWorkflow, nodes, edges },
      selectedNodeId: selectedNodeId === nodeId ? null : selectedNodeId,
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
  selectedEdgeIndex: null,
  selectNode: (id) => set({ selectedNodeId: id, selectedEdgeIndex: null }),
  selectEdge: (index) => set({ selectedEdgeIndex: index, selectedNodeId: null }),
  deselectAll: () => set({ selectedNodeId: null, selectedEdgeIndex: null }),

  // Node registry
  nodeTypes: [],
  loadNodeTypes: async () => {
    const nodeTypes = await api.listNodes();
    set({ nodeTypes });
  },

  // Save
  saveStatus: "idle" as SaveStatus,
  _saveTimer: null,
  _debounceSave: () => {
    const state = get();
    if (state._saveTimer) clearTimeout(state._saveTimer);
    if (state.activeWorkflowPath) {
      get().markDirty(state.activeWorkflowPath);
    }
    const timer = setTimeout(() => {
      get().saveWorkflow();
    }, 300);
    set({ _saveTimer: timer });
  },
  saveWorkflow: async () => {
    const { activeWorkflowPath, activeWorkflow, _rawWorkflow } = get();
    if (!activeWorkflowPath || !activeWorkflow) return;
    set({ saveStatus: "saving" });
    try {
      const payload = denormalizeWorkflow(activeWorkflow, _rawWorkflow ?? {});
      await api.writeFile(activeWorkflowPath, payload);
      set({ saveStatus: "saved" });
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
}));
