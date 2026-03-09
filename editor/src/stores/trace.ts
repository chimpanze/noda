import { create } from "zustand";
import type { TraceEvent, Execution, NodeExecState } from "@/types";

const MAX_EXECUTIONS = 50;

type ConnectionStatus = "disconnected" | "connecting" | "connected";

interface TraceState {
  // Connection
  connectionStatus: ConnectionStatus;
  setConnectionStatus: (status: ConnectionStatus) => void;

  // Executions
  executions: Execution[];
  activeTraceId: string | null;
  setActiveTraceId: (id: string | null) => void;

  // Process incoming event
  processEvent: (event: TraceEvent) => void;
  clearExecutions: () => void;

  // Accessors
  getExecution: (traceId: string) => Execution | undefined;
  getActiveExecution: () => Execution | undefined;
  getNodeState: (traceId: string, nodeId: string) => NodeExecState;
  getNodeData: (traceId: string, nodeId: string) => { output?: string; data?: unknown; error?: string; duration?: string } | undefined;
}

export const useTraceStore = create<TraceState>((set, get) => ({
  connectionStatus: "disconnected",
  setConnectionStatus: (status) => set({ connectionStatus: status }),

  executions: [],
  activeTraceId: null,
  setActiveTraceId: (id) => set({ activeTraceId: id }),

  processEvent: (event) =>
    set((state) => {
      const executions = [...state.executions];
      let exec = executions.find((e) => e.traceId === event.trace_id);

      if (!exec) {
        // New execution
        exec = {
          traceId: event.trace_id,
          workflowId: event.workflow_id,
          status: "running",
          startedAt: event.timestamp,
          events: [],
          nodeStates: new Map(),
          nodeData: new Map(),
        };
        executions.push(exec);
        // Trim to max
        while (executions.length > MAX_EXECUTIONS) executions.shift();
      }

      exec.events.push(event);

      switch (event.type) {
        case "workflow:completed":
          exec.status = "completed";
          exec.duration = event.duration;
          break;
        case "workflow:failed":
          exec.status = "failed";
          exec.duration = event.duration;
          break;
        case "node:entered":
          if (event.node_id) exec.nodeStates.set(event.node_id, "running");
          break;
        case "node:completed":
          if (event.node_id) {
            exec.nodeStates.set(event.node_id, "completed");
            exec.nodeData.set(event.node_id, {
              output: event.output,
              data: event.data,
              duration: event.duration,
            });
          }
          break;
        case "node:failed":
          if (event.node_id) {
            exec.nodeStates.set(event.node_id, "failed");
            exec.nodeData.set(event.node_id, {
              error: event.error,
              duration: event.duration,
            });
          }
          break;
      }

      // Auto-activate latest execution
      return { executions, activeTraceId: exec.traceId };
    }),

  clearExecutions: () => set({ executions: [], activeTraceId: null }),

  getExecution: (traceId) => get().executions.find((e) => e.traceId === traceId),

  getActiveExecution: () => {
    const { executions, activeTraceId } = get();
    return activeTraceId ? executions.find((e) => e.traceId === activeTraceId) : undefined;
  },

  getNodeState: (traceId, nodeId) => {
    const exec = get().executions.find((e) => e.traceId === traceId);
    return exec?.nodeStates.get(nodeId) ?? "idle";
  },

  getNodeData: (traceId, nodeId) => {
    const exec = get().executions.find((e) => e.traceId === traceId);
    return exec?.nodeData.get(nodeId);
  },
}));
