import fs from "fs";
import path from "path";
import type { Plugin } from "vite";

interface DocEntry {
  path: string;
  title: string;
  group: string;
  content: string;
}

function walkDocs(dir: string, group: string, base: string): DocEntry[] {
  const entries: DocEntry[] = [];
  for (const name of fs.readdirSync(dir).sort()) {
    const full = path.join(dir, name);
    const stat = fs.statSync(full);
    if (stat.isDirectory()) {
      const subGroup = name.charAt(0).toUpperCase() + name.slice(1).replace(/-/g, " ");
      entries.push(...walkDocs(full, subGroup, base));
    } else if (name.endsWith(".md")) {
      const content = fs.readFileSync(full, "utf-8");
      const titleMatch = content.match(/^#\s+(.+)$/m);
      const title = titleMatch ? titleMatch[1] : name.replace(/\.md$/, "");
      const relPath = path.relative(base, full);
      entries.push({ path: relPath, title, group, content });
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
        const docs = walkDocs(docsDir, "General", docsDir);
        return `export const docs = ${JSON.stringify(docs)};`;
      }
    },
  };
}
