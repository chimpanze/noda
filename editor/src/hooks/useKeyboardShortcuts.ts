import { useEffect } from "react";
import { useEditorStore } from "@/stores/editor";
import { copyNodes, pasteNodes } from "@/stores/clipboard";
import { autoLayout } from "@/components/canvas/autoLayout";

/**
 * Global keyboard shortcuts for the editor.
 */
export function useKeyboardShortcuts() {
  const undo = useEditorStore((s) => s.undo);
  const redo = useEditorStore((s) => s.redo);
  const saveWorkflow = useEditorStore((s) => s.saveWorkflow);
  const deselectAll = useEditorStore((s) => s.deselectAll);

  useEffect(() => {
    function handler(e: KeyboardEvent) {
      const meta = e.metaKey || e.ctrlKey;

      // Don't intercept when typing in inputs/textareas
      const tag = (e.target as HTMLElement)?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return;

      // Ctrl+Z — undo
      if (meta && e.key === "z" && !e.shiftKey) {
        e.preventDefault();
        undo();
        return;
      }

      // Ctrl+Shift+Z or Ctrl+Y — redo
      if ((meta && e.key === "z" && e.shiftKey) || (meta && e.key === "y")) {
        e.preventDefault();
        redo();
        return;
      }

      // Ctrl+S — save
      if (meta && e.key === "s") {
        e.preventDefault();
        saveWorkflow();
        return;
      }

      // Ctrl+C — copy
      if (meta && e.key === "c") {
        e.preventDefault();
        const state = useEditorStore.getState();
        if (!state.activeWorkflow || !state.selectedNodeId) return;
        copyNodes(
          state.activeWorkflow.nodes,
          state.activeWorkflow.edges,
          new Set([state.selectedNodeId])
        );
        return;
      }

      // Ctrl+V — paste
      if (meta && e.key === "v") {
        e.preventDefault();
        const result = pasteNodes();
        if (!result) return;
        const state = useEditorStore.getState();
        if (!state.activeWorkflow) return;
        for (const node of result.nodes) {
          state.addNode(node);
        }
        for (const edge of result.edges) {
          state.addEdge(edge);
        }
        // Select first pasted node
        if (result.nodes.length > 0) {
          state.selectNode(result.nodes[0].id);
        }
        return;
      }

      // Ctrl+Shift+F — auto-layout
      if (meta && e.key === "f" && e.shiftKey) {
        e.preventDefault();
        const state = useEditorStore.getState();
        if (!state.activeWorkflow) return;
        autoLayout(state.activeWorkflow).then((wf) => {
          state.setWorkflow(wf);
        });
        return;
      }

      // Escape — deselect
      if (e.key === "Escape") {
        deselectAll();
        return;
      }

      // Delete — remove selected
      if (e.key === "Delete" || e.key === "Backspace") {
        const state = useEditorStore.getState();
        if (state.selectedNodeId) {
          e.preventDefault();
          state.removeNode(state.selectedNodeId);
        }
        return;
      }
    }

    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [undo, redo, saveWorkflow, deselectAll]);
}
