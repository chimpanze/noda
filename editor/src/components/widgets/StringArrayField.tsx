import { useCallback } from "react";
import type { FieldProps } from "@rjsf/utils";

export function StringArrayField(props: FieldProps) {
  const { formData, onChange, schema, name, fieldPathId } = props;
  const items: string[] = Array.isArray(formData) ? formData : [];
  const title = schema.title ?? name;
  const path = fieldPathId.path;

  const updateItem = useCallback(
    (index: number, value: string) => {
      const next = [...items];
      next[index] = value;
      onChange(next, path);
    },
    [items, onChange, path]
  );

  const removeItem = useCallback(
    (index: number) => {
      onChange(items.filter((_, i) => i !== index), path);
    },
    [items, onChange, path]
  );

  const addItem = useCallback(() => {
    onChange([...items, ""], path);
  }, [items, onChange, path]);

  return (
    <div className="mb-2">
      <label className="text-sm font-medium text-gray-700 block mb-1">
        {title}
      </label>
      <div className="space-y-1">
        {items.map((item, i) => (
          <div key={i} className="flex items-center gap-1">
            <input
              type="text"
              value={item}
              onChange={(e) => updateItem(i, e.target.value)}
              className="flex-1 px-3 py-1.5 text-sm border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-400 focus:border-transparent"
              placeholder={`Item ${i + 1}`}
            />
            <button
              type="button"
              onClick={() => removeItem(i)}
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
        onClick={addItem}
        className="mt-1 text-xs text-blue-500 hover:text-blue-700"
      >
        + Add item
      </button>
    </div>
  );
}
