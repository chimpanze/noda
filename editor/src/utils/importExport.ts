import type { WorkflowConfig, WorkflowNode } from "@/types";

/**
 * Convert nodes from Noda's map format to array format if needed.
 */
function normalizeNodes(
  raw: unknown
): WorkflowNode[] {
  if (Array.isArray(raw)) return raw as WorkflowNode[];
  if (raw && typeof raw === "object") {
    return Object.entries(raw as Record<string, Record<string, unknown>>).map(
      ([id, node]) => ({
        id,
        type: node.type as string,
        config: node.config as Record<string, unknown> | undefined,
        as: node.as as string | undefined,
        services: node.services as Record<string, string> | undefined,
        position: node.position as { x: number; y: number } | undefined,
      })
    );
  }
  return [];
}

/**
 * Download a workflow as a JSON file.
 */
export function exportWorkflow(workflow: WorkflowConfig, filename: string) {
  // Strip editor-only position data for clean export
  const clean: WorkflowConfig = {
    ...workflow,
    nodes: workflow.nodes.map(({ position: _, ...rest }) => rest),
  };
  const json = JSON.stringify(clean, null, 2);
  downloadFile(json, filename, "application/json");
}

/**
 * Import a workflow from a JSON file. Returns the parsed config or throws on invalid JSON.
 */
export function importWorkflow(file: File): Promise<WorkflowConfig> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      try {
        const data = JSON.parse(reader.result as string);
        // Basic structural validation
        if (!data || typeof data !== "object") {
          throw new Error("File does not contain a valid JSON object");
        }
        if (!data.nodes || (typeof data.nodes !== "object")) {
          throw new Error("Missing or invalid 'nodes' field");
        }
        if (!Array.isArray(data.edges)) {
          throw new Error("Missing or invalid 'edges' array");
        }
        const nodes = normalizeNodes(data.nodes);
        for (const node of nodes) {
          if (!node.id || !node.type) {
            throw new Error(`Node missing required 'id' or 'type' field`);
          }
        }
        for (const edge of data.edges) {
          if (!edge.from || !edge.to) {
            throw new Error(`Edge missing required 'from' or 'to' field`);
          }
        }
        resolve({ nodes, edges: data.edges } as WorkflowConfig);
      } catch (e) {
        reject(e instanceof Error ? e : new Error(String(e)));
      }
    };
    reader.onerror = () => reject(new Error("Failed to read file"));
    reader.readAsText(file);
  });
}

/**
 * Export all project config files as a ZIP.
 */
export async function exportAllAsZip(
  filePaths: string[],
  readFile: (path: string) => Promise<unknown>,
) {
  // Dynamically import fflate for ZIP creation (tree-shakeable, no heavy deps)
  const { zipSync, strToU8 } = await import("fflate");

  const entries: Record<string, Uint8Array> = {};

  await Promise.all(
    filePaths.map(async (path) => {
      try {
        const data = await readFile(path);
        const json = JSON.stringify(data, null, 2);
        entries[path] = strToU8(json);
      } catch {
        // Skip files that can't be read
      }
    }),
  );

  const zipped = zipSync(entries);
  const blob = new Blob([zipped.buffer as ArrayBuffer], { type: "application/zip" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = "noda-project.zip";
  a.click();
  URL.revokeObjectURL(url);
}

function downloadFile(content: string, filename: string, type: string) {
  const blob = new Blob([content], { type });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}
