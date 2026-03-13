import { useCallback, useMemo } from "react";
import type { FieldProps } from "@rjsf/utils";
import { ExpressionAutocomplete } from "./ExpressionAutocomplete";
import { useEditorStore } from "@/stores/editor";

const JOIN_TYPES = ["INNER", "LEFT", "RIGHT", "FULL", "CROSS"];

interface JoinEntry {
  type?: string;
  table?: string;
  on?: string;
}

export function JoinArrayField(props: FieldProps) {
  const { formData, onChange, schema, name, fieldPathId } = props;
  const items: JoinEntry[] = useMemo(
    () => (Array.isArray(formData) ? formData : []),
    [formData],
  );
  const title = schema.title ?? name;
  const path = fieldPathId.path;

  const activeWorkflowPath = useEditorStore((s) => s.activeWorkflowPath);
  const selectedNodeId = useEditorStore((s) => s.selectedNodeId);
  const workflowName = activeWorkflowPath
    ?.replace(/^workflows\//, "")
    .replace(/\.json$/, "");

  const updateItem = useCallback(
    (index: number, patch: Partial<JoinEntry>) => {
      const next = items.map((item, i) =>
        i === index ? { ...item, ...patch } : item,
      );
      onChange(next, path);
    },
    [items, onChange, path],
  );

  const removeItem = useCallback(
    (index: number) => {
      onChange(
        items.filter((_, i) => i !== index),
        path,
      );
    },
    [items, onChange, path],
  );

  const addItem = useCallback(() => {
    onChange([...items, { type: "INNER", table: "", on: "" }], path);
  }, [items, onChange, path]);

  return (
    <div className="mb-2">
      <label className="text-sm font-medium text-gray-700 block mb-1">
        {title}
      </label>
      {schema.description && (
        <p className="text-xs text-gray-400 mb-2">{schema.description}</p>
      )}
      <div className="space-y-2">
        {items.map((item, i) => (
          <div
            key={i}
            className="border border-gray-200 rounded p-2.5 bg-gray-50 space-y-2"
          >
            <div className="flex items-center justify-between">
              <span className="text-[10px] font-medium text-gray-400 uppercase">
                Join #{i + 1}
              </span>
              <button
                type="button"
                onClick={() => removeItem(i)}
                className="text-red-400 hover:text-red-600 text-xs"
                title="Remove join"
              >
                &times;
              </button>
            </div>
            <div className="grid grid-cols-[100px_1fr] gap-2">
              <div>
                <label className="text-[10px] text-gray-400 uppercase block mb-0.5">
                  Type
                </label>
                <select
                  value={item.type ?? "INNER"}
                  onChange={(e) => updateItem(i, { type: e.target.value })}
                  className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-400 focus:border-transparent"
                >
                  {JOIN_TYPES.map((jt) => (
                    <option key={jt} value={jt}>
                      {jt}
                    </option>
                  ))}
                </select>
              </div>
              <div>
                <label className="text-[10px] text-gray-400 uppercase block mb-0.5">
                  Table
                </label>
                <input
                  type="text"
                  value={item.table ?? ""}
                  onChange={(e) => updateItem(i, { table: e.target.value })}
                  className="w-full px-2 py-1.5 text-sm font-mono border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-400 focus:border-transparent"
                  placeholder="e.g. users"
                />
              </div>
            </div>
            <div>
              <label className="text-[10px] text-gray-400 uppercase block mb-0.5">
                ON condition
              </label>
              <ExpressionAutocomplete
                value={item.on ?? ""}
                onChange={(v) => updateItem(i, { on: v })}
                workflow={workflowName}
                node={selectedNodeId ?? undefined}
                className="w-full px-2 py-1.5 text-sm font-mono border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-400 focus:border-transparent"
                placeholder="e.g. tasks.user_id = users.id"
              />
            </div>
          </div>
        ))}
      </div>
      <button
        type="button"
        onClick={addItem}
        className="mt-1.5 text-xs text-blue-500 hover:text-blue-700"
      >
        + Add join
      </button>
    </div>
  );
}
