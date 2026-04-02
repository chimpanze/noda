import type { JSONSchema } from "../types";

export interface SchemaDiff {
  addedKeys: string[];
  removedKeys: string[];
  changedTypes: { key: string; from: string; to: string }[];
  hasChanges: boolean;
}

export function detectSchemaDiff(previous: JSONSchema, current: JSONSchema): SchemaDiff {
  const addedKeys: string[] = [];
  const removedKeys: string[] = [];
  const changedTypes: { key: string; from: string; to: string }[] = [];

  if (previous.type === "object" && current.type === "object") {
    const prevKeys = new Set(Object.keys(previous.properties ?? {}));
    const currKeys = new Set(Object.keys(current.properties ?? {}));

    for (const key of currKeys) {
      if (!prevKeys.has(key)) addedKeys.push(key);
    }
    for (const key of prevKeys) {
      if (!currKeys.has(key)) removedKeys.push(key);
    }

    for (const key of prevKeys) {
      if (!currKeys.has(key)) continue;
      const prevType = previous.properties?.[key]?.type;
      const currType = current.properties?.[key]?.type;
      if (prevType && currType && prevType !== currType) {
        changedTypes.push({ key, from: String(prevType), to: String(currType) });
      }
    }
  } else if (previous.type !== current.type && previous.type && current.type) {
    changedTypes.push({ key: "$root", from: String(previous.type), to: String(current.type) });
  }

  return {
    addedKeys,
    removedKeys,
    changedTypes,
    hasChanges: addedKeys.length > 0 || removedKeys.length > 0 || changedTypes.length > 0,
  };
}

export function diffToLabel(diff: SchemaDiff): string | null {
  if (!diff.hasChanges) return null;
  const parts: string[] = [];
  if (diff.addedKeys.length > 0) parts.push(`+${diff.addedKeys.join(", +")}`);
  if (diff.removedKeys.length > 0) parts.push(`-${diff.removedKeys.join(", -")}`);
  if (diff.changedTypes.length > 0) {
    parts.push(diff.changedTypes.map((c) => `${c.key}: ${c.from}→${c.to}`).join(", "));
  }
  return parts.join("; ");
}
