import fs from "fs";
import path from "path";
import type { Plugin } from "vite";

interface DocEntry {
  path: string;
  title: string;
  group: string;
  content: string;
  nodeType?: string;
  sortOrder?: number;
}

function walkDocs(dir: string, group: string, base: string, sortOrder: number): DocEntry[] {
  const entries: DocEntry[] = [];
  for (const name of fs.readdirSync(dir).sort()) {
    // Skip directories and files starting with underscore
    if (name.startsWith("_") && fs.statSync(path.join(dir, name)).isDirectory()) continue;

    const full = path.join(dir, name);
    const stat = fs.statSync(full);
    if (stat.isDirectory()) {
      // Strip numeric prefix for display: "01-getting-started" → "Getting started"
      const displayName = name.replace(/^\d+-/, "").replace(/-/g, " ");
      const subGroup = displayName.charAt(0).toUpperCase() + displayName.slice(1);
      // Extract sort order from numeric prefix
      const dirOrder = parseInt(name.match(/^(\d+)/)?.[1] ?? "99", 10);
      entries.push(...walkDocs(full, subGroup, base, dirOrder));
    } else if (name.endsWith(".md")) {
      const content = fs.readFileSync(full, "utf-8");
      const titleMatch = content.match(/^#\s+(.+)$/m);
      const title = titleMatch ? titleMatch[1] : name.replace(/\.md$/, "");
      const relPath = path.relative(base, full);

      // Extract nodeType from files in the nodes directory (e.g., "db.query.md" → "db.query")
      let nodeType: string | undefined;
      const parentDir = path.basename(path.dirname(full));
      if (parentDir.match(/nodes$/) || parentDir.match(/^\d+-nodes$/)) {
        if (!name.startsWith("_")) {
          nodeType = name.replace(/\.md$/, "");
        }
      }

      // _index.md files sort first within their group
      const isIndex = name === "_index.md";

      entries.push({
        path: relPath,
        title,
        group,
        content,
        ...(nodeType ? { nodeType } : {}),
        sortOrder: isIndex ? sortOrder - 0.5 : sortOrder,
      });
    }
  }
  return entries;
}

export default function docsPlugin(): Plugin {
  const virtualModuleId = "virtual:docs";
  const resolvedVirtualModuleId = "\0" + virtualModuleId;

  return {
    name: "vite-plugin-docs",
    resolveId(id) {
      if (id === virtualModuleId) return resolvedVirtualModuleId;
    },
    load(id) {
      if (id === resolvedVirtualModuleId) {
        const docsDir = path.resolve(__dirname, "..", "docs");
        const docs = walkDocs(docsDir, "General", docsDir, 0);

        // Build node type → doc path index
        const nodeDocIndex: Record<string, string> = {};
        for (const doc of docs) {
          if (doc.nodeType) {
            nodeDocIndex[doc.nodeType] = doc.path;
          }
        }

        return [
          `export const docs = ${JSON.stringify(docs)};`,
          `export const nodeDocIndex = ${JSON.stringify(nodeDocIndex)};`,
        ].join("\n");
      }
    },
  };
}
