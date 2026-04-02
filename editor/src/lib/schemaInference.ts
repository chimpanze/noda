import type { JSONSchema } from "../types";

/**
 * Derive a JSON Schema from a concrete runtime value.
 */
export function inferSchema(data: unknown): JSONSchema {
  if (data === null || data === undefined) {
    return { type: "null" };
  }

  if (typeof data === "string") {
    return { type: "string" };
  }

  if (typeof data === "number") {
    return { type: "number" };
  }

  if (typeof data === "boolean") {
    return { type: "boolean" };
  }

  if (Array.isArray(data)) {
    if (data.length === 0) {
      return { type: "array" };
    }
    return { type: "array", items: inferSchema(data[0]) };
  }

  if (typeof data === "object") {
    const obj = data as Record<string, unknown>;
    const keys = Object.keys(obj);
    const properties: Record<string, JSONSchema> = {};
    for (const key of keys) {
      properties[key] = inferSchema(obj[key]);
    }
    return { type: "object", properties, required: keys };
  }

  return { type: "null" };
}

/**
 * Merge two schemas, widening where types differ.
 */
export function mergeSchemas(
  existing: JSONSchema,
  incoming: JSONSchema,
): JSONSchema {
  // If types differ, produce anyOf
  if (existing.type !== incoming.type) {
    // If one of them is already an anyOf, flatten it
    const existingVariants = existing.anyOf ?? [existing];
    const incomingVariants = incoming.anyOf ?? [incoming];

    // Deduplicate by type
    const seen = new Set<string | undefined>();
    const merged: JSONSchema[] = [];
    for (const v of [...existingVariants, ...incomingVariants]) {
      const key = JSON.stringify(v.type);
      if (!seen.has(key)) {
        seen.add(key);
        merged.push(v);
      }
    }

    if (merged.length === 1) {
      return merged[0];
    }
    return { anyOf: merged };
  }

  // Both are anyOf — merge the variant lists
  if (existing.anyOf && incoming.anyOf) {
    const seen = new Set<string>();
    const merged: JSONSchema[] = [];
    for (const v of [...existing.anyOf, ...incoming.anyOf]) {
      const key = JSON.stringify(v.type);
      if (!seen.has(key)) {
        seen.add(key);
        merged.push(v);
      }
    }
    return { anyOf: merged };
  }

  // Both objects — merge properties, required = intersection
  if (existing.type === "object" && incoming.type === "object") {
    const existingProps = existing.properties ?? {};
    const incomingProps = incoming.properties ?? {};
    const allKeys = new Set([
      ...Object.keys(existingProps),
      ...Object.keys(incomingProps),
    ]);

    const mergedProps: Record<string, JSONSchema> = {};
    for (const key of allKeys) {
      if (existingProps[key] && incomingProps[key]) {
        mergedProps[key] = mergeSchemas(existingProps[key], incomingProps[key]);
      } else {
        mergedProps[key] = existingProps[key] ?? incomingProps[key];
      }
    }

    // required = intersection of both required arrays
    const existingRequired = new Set(existing.required ?? []);
    const incomingRequired = new Set(incoming.required ?? []);
    const required = [...existingRequired].filter((k) =>
      incomingRequired.has(k),
    );

    return {
      type: "object",
      properties: mergedProps,
      ...(required.length > 0 ? { required } : {}),
    };
  }

  // Both arrays — merge items
  if (existing.type === "array" && incoming.type === "array") {
    if (existing.items && incoming.items) {
      return { type: "array", items: mergeSchemas(existing.items, incoming.items) };
    }
    return { type: "array", ...(existing.items ?? incoming.items ? { items: existing.items ?? incoming.items } : {}) };
  }

  // Same primitive type — return incoming
  return incoming;
}

/**
 * Generate a compact human-readable label from a JSON Schema.
 */
export function schemaToCompactLabel(schema: JSONSchema): string {
  if (schema.anyOf && schema.anyOf.length > 0) {
    return schema.anyOf.map(schemaToCompactLabel).join(" | ");
  }

  if (schema.type === "object") {
    const keys = Object.keys(schema.properties ?? {});
    if (keys.length === 0) {
      return "{}";
    }
    if (keys.length <= 2) {
      return `{${keys.join(", ")}}`;
    }
    const extra = keys.length - 2;
    return `{${keys[0]}, ${keys[1]}, +${extra}}`;
  }

  if (schema.type === "array") {
    if (schema.items) {
      return `[${schemaToCompactLabel(schema.items)}]`;
    }
    return "[]";
  }

  if (schema.type) {
    return Array.isArray(schema.type) ? schema.type.join(" | ") : schema.type;
  }

  return "unknown";
}

/**
 * Generate a compact human-readable label from an actual runtime value.
 */
export function dataToCompactLabel(data: unknown): string {
  if (data === null || data === undefined) {
    return "null";
  }

  if (typeof data === "string") {
    if (data.length <= 20) {
      return `"${data}"`;
    }
    return `"${data.slice(0, 17)}..."`;
  }

  if (typeof data === "number" || typeof data === "boolean") {
    return String(data);
  }

  if (Array.isArray(data)) {
    return `[${data.length} items]`;
  }

  if (typeof data === "object") {
    const obj = data as Record<string, unknown>;
    const keys = Object.keys(obj);
    if (keys.length === 0) {
      return "{}";
    }
    if (keys.length <= 2) {
      return `{${keys.join(", ")}}`;
    }
    const extra = keys.length - 2;
    return `{${keys[0]}, ${keys[1]}, +${extra}}`;
  }

  return "null";
}
