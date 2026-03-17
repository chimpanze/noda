import { useState, useCallback, useEffect } from "react";
import * as api from "@/api/client";
import { useEditorStore } from "@/stores/editor";

interface UseConfigSectionOptions<T> {
  path: string;
  parse?: (raw: unknown) => T;
  serialize?: (value: T) => unknown;
}

interface UseConfigSectionResult<T> {
  data: T;
  rootConfig: Record<string, unknown>;
  loading: boolean;
  saving: boolean;
  set: (key: string, value: unknown) => Promise<void>;
  remove: (key: string) => Promise<void>;
  replace: (value: T) => Promise<void>;
  reload: () => Promise<void>;
}

function getPath(obj: Record<string, unknown>, path: string): unknown {
  return path.split(".").reduce<unknown>((o, k) => {
    if (o && typeof o === "object") return (o as Record<string, unknown>)[k];
    return undefined;
  }, obj);
}

function setPath(
  obj: Record<string, unknown>,
  path: string,
  value: unknown,
): void {
  const keys = path.split(".");
  let current: Record<string, unknown> = obj;
  for (let i = 0; i < keys.length - 1; i++) {
    if (!current[keys[i]] || typeof current[keys[i]] !== "object") {
      current[keys[i]] = {};
    }
    current = current[keys[i]] as Record<string, unknown>;
  }
  current[keys[keys.length - 1]] = value;
}

function deletePath(obj: Record<string, unknown>, path: string): void {
  const keys = path.split(".");
  let current: Record<string, unknown> = obj;
  for (let i = 0; i < keys.length - 1; i++) {
    if (!current[keys[i]] || typeof current[keys[i]] !== "object") return;
    current = current[keys[i]] as Record<string, unknown>;
  }
  delete current[keys[keys.length - 1]];
}

export function useConfigSection<T = Record<string, unknown>>(
  options: UseConfigSectionOptions<T>,
): UseConfigSectionResult<T> {
  const { path, parse, serialize } = options;
  const files = useEditorStore((s) => s.files);
  const [rootConfig, setRootConfig] = useState<Record<string, unknown>>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  const reload = useCallback(async () => {
    const root = files?.root;
    if (!root) {
      setLoading(false);
      return;
    }
    setLoading(true);
    try {
      const data = (await api.readFile(root)) as Record<string, unknown>;
      setRootConfig(data);
    } finally {
      setLoading(false);
    }
  }, [files?.root]);

  useEffect(() => {
    reload();
  }, [reload]);

  const rawData = getPath(rootConfig, path);
  const data: T = parse
    ? parse(rawData)
    : ((rawData ?? {}) as T);

  const writeRoot = useCallback(
    async (updated: Record<string, unknown>) => {
      const root = files?.root;
      if (!root) return;
      setSaving(true);
      try {
        await api.writeFile(root, updated);
        setRootConfig(updated);
      } finally {
        setSaving(false);
      }
    },
    [files?.root],
  );

  const set = useCallback(
    async (key: string, value: unknown) => {
      const updated = structuredClone(rootConfig);
      const section = (getPath(updated, path) ?? {}) as Record<string, unknown>;
      section[key] = value;
      setPath(updated, path, section);
      await writeRoot(updated);
    },
    [rootConfig, path, writeRoot],
  );

  const remove = useCallback(
    async (key: string) => {
      const updated = structuredClone(rootConfig);
      const section = getPath(updated, path) as Record<string, unknown> | undefined;
      if (!section) return;
      delete section[key];
      if (Object.keys(section).length === 0) {
        deletePath(updated, path);
      } else {
        setPath(updated, path, section);
      }
      await writeRoot(updated);
    },
    [rootConfig, path, writeRoot],
  );

  const replace = useCallback(
    async (value: T) => {
      const updated = structuredClone(rootConfig);
      const serialized = serialize ? serialize(value) : value;
      setPath(updated, path, serialized);
      await writeRoot(updated);
    },
    [rootConfig, path, serialize, writeRoot],
  );

  return { data, rootConfig, loading, saving, set, remove, replace, reload };
}
