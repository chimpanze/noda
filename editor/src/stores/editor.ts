import { create } from "zustand";
import type {
  ViewType,
  FileList,
  WorkflowConfig,
  WorkflowNode,
  WorkflowEdge,
  TraceEvent,
  ValidationError,
  NodeDescriptor,
} from "@/types";
import * as api from "@/api/client";

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
  setActiveWorkflow: (path: string | null) => void;
  loadWorkflow: (path: string) => Promise<void>;

  // Workflow mutations
  updateNodeConfig: (nodeId: string, config: Record<string, unknown>) => void;
  updateNodeServices: (nodeId: string, services: Record<string, string>) => void;
  addNode: (node: WorkflowNode) => void;
  removeNode: (nodeId: string) => void;
  updateNodePosition: (nodeId: string, position: { x: number; y: number }) => void;
  addEdge: (edge: WorkflowEdge) => void;
  removeEdge: (from: string, output: string, to: string) => void;

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

  // Traces
  traces: TraceEvent[];
  addTrace: (event: TraceEvent) => void;
  clearTraces: () => void;

  // Validation
  validationErrors: ValidationError[];
  setValidationErrors: (errors: ValidationError[]) => void;

  // Dirty state
  dirtyFiles: Set<string>;
  markDirty: (path: string) => void;
  markClean: (path: string) => void;
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
  setActiveWorkflow: (path) => {
    if (path === null) {
      set({ activeWorkflowPath: null, activeWorkflow: null, selectedNodeId: null, selectedEdgeIndex: null });
    } else {
      get().loadWorkflow(path);
    }
  },
  loadWorkflow: async (path) => {
    const data = (await api.readFile(path)) as WorkflowConfig;
    set({
      activeWorkflowPath: path,
      activeWorkflow: data,
      selectedNodeId: null,
      selectedEdgeIndex: null,
    });
  },

  // Workflow mutations
  updateNodeConfig: (nodeId, config) =>
    set((state) => {
      if (!state.activeWorkflow) return state;
      const nodes = state.activeWorkflow.nodes.map((n) =>
        n.id === nodeId ? { ...n, config } : n
      );
      const wf = { ...state.activeWorkflow, nodes };
      get()._debounceSave();
      return { activeWorkflow: wf };
    }),

  updateNodeServices: (nodeId, services) =>
    set((state) => {
      if (!state.activeWorkflow) return state;
      const nodes = state.activeWorkflow.nodes.map((n) =>
        n.id === nodeId ? { ...n, services } : n
      );
      const wf = { ...state.activeWorkflow, nodes };
      get()._debounceSave();
      return { activeWorkflow: wf };
    }),

  addNode: (node) =>
    set((state) => {
      if (!state.activeWorkflow) return state;
      const nodes = [...state.activeWorkflow.nodes, node];
      const wf = { ...state.activeWorkflow, nodes };
      get()._debounceSave();
      return { activeWorkflow: wf };
    }),

  removeNode: (nodeId) =>
    set((state) => {
      if (!state.activeWorkflow) return state;
      const nodes = state.activeWorkflow.nodes.filter((n) => n.id !== nodeId);
      const edges = state.activeWorkflow.edges.filter(
        (e) => e.from !== nodeId && e.to !== nodeId
      );
      const wf = { ...state.activeWorkflow, nodes, edges };
      const selectedNodeId = state.selectedNodeId === nodeId ? null : state.selectedNodeId;
      get()._debounceSave();
      return { activeWorkflow: wf, selectedNodeId };
    }),

  updateNodePosition: (nodeId, position) =>
    set((state) => {
      if (!state.activeWorkflow) return state;
      const nodes = state.activeWorkflow.nodes.map((n) =>
        n.id === nodeId ? { ...n, position } : n
      );
      return { activeWorkflow: { ...state.activeWorkflow, nodes } };
    }),

  addEdge: (edge) =>
    set((state) => {
      if (!state.activeWorkflow) return state;
      // Prevent duplicates
      const exists = state.activeWorkflow.edges.some(
        (e) => e.from === edge.from && e.output === edge.output && e.to === edge.to
      );
      if (exists) return state;
      const edges = [...state.activeWorkflow.edges, edge];
      const wf = { ...state.activeWorkflow, edges };
      get()._debounceSave();
      return { activeWorkflow: wf };
    }),

  removeEdge: (from, output, to) =>
    set((state) => {
      if (!state.activeWorkflow) return state;
      const edges = state.activeWorkflow.edges.filter(
        (e) => !(e.from === from && e.output === output && e.to === to)
      );
      const wf = { ...state.activeWorkflow, edges };
      get()._debounceSave();
      return { activeWorkflow: wf };
    }),

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
    const { activeWorkflowPath, activeWorkflow } = get();
    if (!activeWorkflowPath || !activeWorkflow) return;
    set({ saveStatus: "saving" });
    try {
      await api.writeFile(activeWorkflowPath, activeWorkflow);
      set({ saveStatus: "saved" });
      get().markClean(activeWorkflowPath);
      setTimeout(() => {
        if (get().saveStatus === "saved") set({ saveStatus: "idle" });
      }, 2000);
    } catch {
      set({ saveStatus: "error" });
    }
  },

  // Traces
  traces: [],
  addTrace: (event) =>
    set((state) => ({ traces: [...state.traces.slice(-99), event] })),
  clearTraces: () => set({ traces: [] }),

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
