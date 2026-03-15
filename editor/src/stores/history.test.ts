import { describe, it, expect, beforeEach } from "vitest";
import {
  pushSnapshot,
  undo,
  redo,
  clearHistory,
  canUndo,
  canRedo,
} from "./history";
import type { WorkflowConfig } from "@/types";

function makeWorkflow(desc: string): WorkflowConfig {
  return { nodes: [], edges: [], description: desc };
}

describe("history", () => {
  const path = "test/workflow.json";

  beforeEach(() => {
    clearHistory(path);
  });

  it("starts with no undo/redo", () => {
    expect(canUndo(path)).toBe(false);
    expect(canRedo(path)).toBe(false);
  });

  it("undo returns previous state", () => {
    const v1 = makeWorkflow("v1");
    const v2 = makeWorkflow("v2");
    pushSnapshot(path, v1);
    const result = undo(path, v2);
    expect(result?.description).toBe("v1");
  });

  it("redo returns next state after undo", () => {
    const v1 = makeWorkflow("v1");
    const v2 = makeWorkflow("v2");
    pushSnapshot(path, v1);
    undo(path, v2);
    const result = redo(path, v1);
    expect(result?.description).toBe("v2");
  });

  it("undo returns null when nothing to undo", () => {
    expect(undo(path, makeWorkflow("current"))).toBeNull();
  });

  it("redo returns null when nothing to redo", () => {
    expect(redo(path, makeWorkflow("current"))).toBeNull();
  });

  it("new edit clears redo stack", () => {
    const v1 = makeWorkflow("v1");
    const v2 = makeWorkflow("v2");
    const v3 = makeWorkflow("v3");
    pushSnapshot(path, v1);
    undo(path, v2);
    expect(canRedo(path)).toBe(true);
    pushSnapshot(path, v3);
    expect(canRedo(path)).toBe(false);
  });

  it("respects MAX_HISTORY limit of 50", () => {
    for (let i = 0; i < 60; i++) {
      pushSnapshot(path, makeWorkflow(`v${i}`));
    }
    // Should only be able to undo 50 times
    let count = 0;
    let current = makeWorkflow("current");
    while (true) {
      const prev = undo(path, current);
      if (!prev) break;
      current = prev;
      count++;
    }
    expect(count).toBe(50);
  });

  it("clearHistory resets state", () => {
    pushSnapshot(path, makeWorkflow("v1"));
    expect(canUndo(path)).toBe(true);
    clearHistory(path);
    expect(canUndo(path)).toBe(false);
  });

  it("creates deep copies to avoid mutation", () => {
    const wf = makeWorkflow("v1");
    wf.nodes.push({ id: "n1", type: "test" });
    pushSnapshot(path, wf);
    wf.nodes[0].id = "mutated";
    const restored = undo(path, makeWorkflow("v2"));
    expect(restored?.nodes[0].id).toBe("n1");
  });
});
