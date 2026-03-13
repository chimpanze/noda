import type { SchemaInfo } from "@/types";

export function groupByFolder(
  schemas: SchemaInfo[],
): Map<string, SchemaInfo[]> {
  const groups = new Map<string, SchemaInfo[]>();
  for (const s of schemas) {
    const withoutPrefix = s.path.replace(/^schemas\//, "");
    const lastSlash = withoutPrefix.lastIndexOf("/");
    const folder = lastSlash >= 0 ? withoutPrefix.substring(0, lastSlash) : "";
    const group = groups.get(folder) ?? [];
    group.push(s);
    groups.set(folder, group);
  }
  return groups;
}
