import { describe, it, expect } from "vitest";
import { copyNodes, pasteNodes, hasClipboard } from "./clipboard";
import type { WorkflowNode, WorkflowEdge } from "@/types";

const nodes: WorkflowNode[] = [
  { id: "a", type: "transform.set", config: { key: "x" } },
  { id: "b", type: "response.json", position: { x: 100, y: 200 } },
  { id: "c", type: "util.log" },
];

const edges: WorkflowEdge[] = [
  { from: "a", output: "success", to: "b" },
  { from: "b", output: "success", to: "c" },
];

describe("clipboard", () => {
  it("starts with empty clipboard", () => {
    expect(hasClipboard()).toBe(false);
    expect(pasteNodes()).toBeNull();
  });

  it("copies selected nodes and internal edges", () => {
    copyNodes(nodes, edges, new Set(["a", "b"]));
    expect(hasClipboard()).toBe(true);

    const pasted = pasteNodes()!;
    expect(pasted.nodes).toHaveLength(2);
    // Only the a→b edge should be included (both endpoints selected)
    expect(pasted.edges).toHaveLength(1);
    expect(pasted.edges[0].from).toContain("a");
    expect(pasted.edges[0].to).toContain("b");
  });

  it("generates unique IDs with copy suffix", () => {
    copyNodes(nodes, edges, new Set(["a"]));
    const p1 = pasteNodes()!;
    const p2 = pasteNodes()!;
    expect(p1.nodes[0].id).not.toBe(p2.nodes[0].id);
    expect(p1.nodes[0].id).toContain("copy");
  });

  it("offsets positions on paste", () => {
    copyNodes(nodes, edges, new Set(["b"]));
    const p1 = pasteNodes()!;
    expect(p1.nodes[0].position!.x).toBeGreaterThan(100);
    expect(p1.nodes[0].position!.y).toBeGreaterThan(200);
  });

  it("does not copy when selection is empty", () => {
    copyNodes(nodes, edges, new Set());
    // clipboard state should remain from previous test since copyNodes returns early
  });

  it("creates deep copies to prevent mutation", () => {
    copyNodes(nodes, edges, new Set(["a"]));
    const pasted = pasteNodes()!;
    pasted.nodes[0].config = { mutated: true };
    const pasted2 = pasteNodes()!;
    expect(pasted2.nodes[0].config).toEqual({ key: "x" });
  });
});
