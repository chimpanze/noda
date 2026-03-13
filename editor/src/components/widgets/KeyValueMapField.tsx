import { useCallback, useMemo } from "react";
import type { FieldProps } from "@rjsf/utils";

export function KeyValueMapField(props: FieldProps) {
  const { formData, onChange, schema, name, fieldPathId } = props;
  const data: Record<string, string> = useMemo(
    () =>
      formData && typeof formData === "object" && !Array.isArray(formData)
        ? (formData as Record<string, string>)
        : {},
    [formData],
  );
  const entries = Object.entries(data);
  const title = schema.title ?? name;
  const path = fieldPathId.path;

  const updateKey = useCallback(
    (oldKey: string, newKey: string) => {
      const next: Record<string, string> = {};
      for (const [k, v] of entries) {
        next[k === oldKey ? newKey : k] = v;
      }
      onChange(next, path);
    },
    [entries, onChange, path],
  );

  const updateValue = useCallback(
    (key: string, value: string) => {
      onChange({ ...data, [key]: value }, path);
    },
    [data, onChange, path],
  );

  const removeEntry = useCallback(
    (key: string) => {
      const next = { ...data };
      delete next[key];
      onChange(next, path);
    },
    [data, onChange, path],
  );

  const addEntry = useCallback(() => {
    let newKey = "key";
    let counter = 1;
    while (newKey in data) {
      newKey = `key${counter++}`;
    }
    onChange({ ...data, [newKey]: "" }, path);
  }, [data, onChange, path]);

  return (
    <div className="mb-2">
      <label className="text-sm font-medium text-gray-700 block mb-1">
        {title}
      </label>
      <div className="space-y-1">
        {entries.map(([key, val]) => (
          <div key={key} className="flex items-center gap-1">
            <input
              type="text"
              value={key}
              onChange={(e) => updateKey(key, e.target.value)}
              className="w-1/3 px-2 py-1.5 text-sm border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-400 focus:border-transparent font-mono"
              placeholder="key"
            />
            <span className="text-gray-400 text-xs">:</span>
            <input
              type="text"
              value={val}
              onChange={(e) => updateValue(key, e.target.value)}
              className="flex-1 px-2 py-1.5 text-sm border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-400 focus:border-transparent"
              placeholder="value or {{ expression }}"
            />
            <button
              type="button"
              onClick={() => removeEntry(key)}
              className="px-2 py-1.5 text-sm text-red-400 hover:text-red-600"
              title="Remove"
            >
              &times;
            </button>
          </div>
        ))}
      </div>
      <button
        type="button"
        onClick={addEntry}
        className="mt-1 text-xs text-blue-500 hover:text-blue-700"
      >
        + Add field
      </button>
    </div>
  );
}
