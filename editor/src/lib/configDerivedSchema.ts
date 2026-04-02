import type { JSONSchema } from "../types";

type SchemaDeriver = (config: Record<string, unknown>) => JSONSchema | null;

const derivers: Record<string, SchemaDeriver> = {
  "transform.set": (config) => {
    const fields = config.fields;
    if (!fields || typeof fields !== "object") return null;
    const properties: Record<string, JSONSchema> = {};
    for (const key of Object.keys(fields as Record<string, unknown>)) {
      properties[key] = {};
    }
    return { type: "object", properties };
  },

  "db.create": (config) => {
    const values = config.values;
    if (!values || typeof values !== "object") return null;
    const properties: Record<string, JSONSchema> = {};
    for (const key of Object.keys(values as Record<string, unknown>)) {
      properties[key] = {};
    }
    return { type: "object", properties };
  },

  "db.update": (config) => {
    const values = config.values;
    if (!values || typeof values !== "object") return null;
    const properties: Record<string, JSONSchema> = {};
    for (const key of Object.keys(values as Record<string, unknown>)) {
      properties[key] = {};
    }
    return { type: "object", properties };
  },
};

export function deriveSchemaFromConfig(
  nodeType: string,
  config: Record<string, unknown>,
): JSONSchema | null {
  const deriver = derivers[nodeType];
  if (!deriver) return null;
  return deriver(config);
}
