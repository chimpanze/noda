import { create } from "zustand";
import type {
  ViewType,
  FileList,
  WorkflowConfig,
  TraceEvent,
  ValidationError,
  NodeDescriptor,
} from "@/types";
import * as api from "@/api/client";

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

  // Selection
  selectedNodeId: string | null;
  selectedEdgeIndex: number | null;
  selectNode: (id: string | null) => void;
  selectEdge: (index: number | null) => void;
  deselectAll: () => void;

  // Node registry
  nodeTypes: NodeDescriptor[];
  loadNodeTypes: () => Promise<void>;

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
