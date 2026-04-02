import { create } from "zustand";
import type { JSONSchema, OutputSchema } from "../types";
import { fetchOutputSchemas } from "../api/client";
import { inferSchema, mergeSchemas } from "../lib/schemaInference";
import { deriveSchemaFromConfig } from "../lib/configDerivedSchema";

const STORAGE_PREFIX = "noda:schema:";

interface SchemaState {
  staticSchemas: Record<string, JSONSchema>;
  loadStaticSchemas: () => Promise<void>;
  getNodeOutputSchema: (
    nodeId: string,
    nodeType: string,
    config?: Record<string, unknown>,
  ) => OutputSchema | null;
  updateLearnedSchema: (
    workflowId: string,
    nodeId: string,
    data: unknown,
  ) => void;
  clearLearnedSchemas: (workflowId: string) => void;
  markStale: (workflowId: string, nodeId: string) => void;
  getPreviousSchema: (nodeId: string) => JSONSchema | null;
  _learnedSchemas: Record<string, JSONSchema>;
  _previousSchemas: Record<string, JSONSchema>;
  _staleKeys: Set<string>;
}

export function loadLearnedSchemasFromStorage(
  workflowId: string,
): Record<string, JSONSchema> {
  const prefix = `${STORAGE_PREFIX}${workflowId}:`;
  const result: Record<string, JSONSchema> = {};
  for (let i = 0; i < localStorage.length; i++) {
    const key = localStorage.key(i);
    if (key && key.startsWith(prefix)) {
      const nodeId = key.slice(prefix.length);
      try {
        const raw = localStorage.getItem(key);
        if (raw) {
          result[nodeId] = JSON.parse(raw) as JSONSchema;
        }
      } catch {
        // ignore malformed entries
      }
    }
  }
  return result;
}

export const useSchemaStore = create<SchemaState>((set, get) => ({
  staticSchemas: {},
  _learnedSchemas: {},
  _previousSchemas: {},
  _staleKeys: new Set<string>(),

  loadStaticSchemas: async () => {
    try {
      const schemas = await fetchOutputSchemas();
      set({ staticSchemas: schemas as Record<string, JSONSchema> });
    } catch {
      // Non-fatal — static schemas are best-effort
    }
  },

  getNodeOutputSchema: (nodeId, nodeType, config) => {
    const { staticSchemas, _learnedSchemas, _staleKeys } = get();

    // Priority 1: static schema from the registry
    if (staticSchemas[nodeType]) {
      return {
        schema: staticSchemas[nodeType],
        source: "static",
        stale: false,
      };
    }

    // Priority 2: config-derived schema
    if (config) {
      const derived = deriveSchemaFromConfig(nodeType, config);
      if (derived) {
        return {
          schema: derived,
          source: "config-derived",
          stale: false,
        };
      }
    }

    // Priority 3: runtime-learned schema from trace data
    if (_learnedSchemas[nodeId]) {
      return {
        schema: _learnedSchemas[nodeId],
        source: "runtime-learned",
        stale: _staleKeys.has(nodeId),
      };
    }

    return null;
  },

  updateLearnedSchema: (workflowId, nodeId, data) => {
    const inferred = inferSchema(data);
    set((state) => {
      const existing = state._learnedSchemas[nodeId];
      const merged = existing ? mergeSchemas(existing, inferred) : inferred;

      // Persist to localStorage
      const storageKey = `${STORAGE_PREFIX}${workflowId}:${nodeId}`;
      try {
        localStorage.setItem(storageKey, JSON.stringify(merged));
      } catch {
        // ignore storage errors (e.g. quota exceeded)
      }

      // Remove from stale set now that it has fresh data
      const newStaleKeys = new Set(state._staleKeys);
      newStaleKeys.delete(nodeId);

      // Save previous schema before overwriting
      const previous = state._learnedSchemas[nodeId];

      return {
        _learnedSchemas: { ...state._learnedSchemas, [nodeId]: merged },
        _previousSchemas: previous
          ? { ...state._previousSchemas, [nodeId]: previous }
          : state._previousSchemas,
        _staleKeys: newStaleKeys,
      };
    });
  },

  clearLearnedSchemas: (workflowId) => {
    const prefix = `${STORAGE_PREFIX}${workflowId}:`;
    const keysToRemove: string[] = [];
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i);
      if (key && key.startsWith(prefix)) {
        keysToRemove.push(key);
      }
    }
    for (const key of keysToRemove) {
      localStorage.removeItem(key);
    }

    set((state) => {
      // Remove all learned schemas whose storage key belonged to this workflow
      const newLearnedSchemas = { ...state._learnedSchemas };
      for (const key of keysToRemove) {
        const nodeId = key.slice(prefix.length);
        delete newLearnedSchemas[nodeId];
      }
      return { _learnedSchemas: newLearnedSchemas };
    });
  },

  getPreviousSchema: (nodeId) => {
    return get()._previousSchemas[nodeId] ?? null;
  },

  markStale: (_workflowId, nodeId) => {
    // _staleKeys is scoped to the current workflow — reset on workflow switch in editor.ts loadWorkflow
    set((state) => {
      const newStaleKeys = new Set(state._staleKeys);
      newStaleKeys.add(nodeId);
      return { _staleKeys: newStaleKeys };
    });
  },
}));
