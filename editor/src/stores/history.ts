import type { WorkflowConfig } from "@/types";

const MAX_HISTORY = 50;

interface HistoryState {
  past: WorkflowConfig[];
  future: WorkflowConfig[];
}

const histories = new Map<string, HistoryState>();

function getHistory(path: string): HistoryState {
  if (!histories.has(path)) {
    histories.set(path, { past: [], future: [] });
  }
  return histories.get(path)!;
}

/** Push a snapshot before mutation. */
export function pushSnapshot(path: string, workflow: WorkflowConfig) {
  const h = getHistory(path);
  h.past.push(structuredClone(workflow));
  if (h.past.length > MAX_HISTORY) h.past.shift();
  // Any new edit clears the redo stack
  h.future.length = 0;
}

/** Undo: returns the previous workflow state, or null if nothing to undo. */
export function undo(
  path: string,
  current: WorkflowConfig,
): WorkflowConfig | null {
  const h = getHistory(path);
  if (h.past.length === 0) return null;
  h.future.push(structuredClone(current));
  return h.past.pop()!;
}

/** Redo: returns the next workflow state, or null if nothing to redo. */
export function redo(
  path: string,
  current: WorkflowConfig,
): WorkflowConfig | null {
  const h = getHistory(path);
  if (h.future.length === 0) return null;
  h.past.push(structuredClone(current));
  return h.future.pop()!;
}

/** Clear history for a path (e.g., on workflow load). */
export function clearHistory(path: string) {
  histories.delete(path);
}

export function canUndo(path: string): boolean {
  return (histories.get(path)?.past.length ?? 0) > 0;
}

export function canRedo(path: string): boolean {
  return (histories.get(path)?.future.length ?? 0) > 0;
}
