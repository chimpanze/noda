import { useCallback, useMemo } from "react";
import type { FieldProps } from "@rjsf/utils";
import { ExpressionAutocomplete } from "./ExpressionAutocomplete";
import { useEditorStore } from "@/stores/editor";

const SAME_SITE_OPTIONS = ["", "Strict", "Lax", "None"];

interface CookieEntry {
  name?: string;
  value?: string;
  path?: string;
  domain?: string;
  max_age?: number;
  secure?: boolean;
  http_only?: boolean;
  same_site?: string;
}

export function CookieArrayField(props: FieldProps) {
  const { formData, onChange, schema, name, fieldPathId } = props;
  const items: CookieEntry[] = useMemo(
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
    (index: number, patch: Partial<CookieEntry>) => {
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
    onChange(
      [
        ...items,
        { name: "", value: "", path: "/", secure: false, http_only: false },
      ],
      path,
    );
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
                Cookie #{i + 1}
              </span>
              <button
                type="button"
                onClick={() => removeItem(i)}
                className="text-red-400 hover:text-red-600 text-xs"
                title="Remove cookie"
              >
                &times;
              </button>
            </div>

            {/* Name + Value */}
            <div className="grid grid-cols-2 gap-2">
              <div>
                <label className="text-[10px] text-gray-400 uppercase block mb-0.5">
                  Name
                </label>
                <input
                  type="text"
                  value={item.name ?? ""}
                  onChange={(e) => updateItem(i, { name: e.target.value })}
                  className="w-full px-2 py-1.5 text-sm font-mono border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-400 focus:border-transparent"
                  placeholder="e.g. session"
                />
              </div>
              <div>
                <label className="text-[10px] text-gray-400 uppercase block mb-0.5">
                  Value
                </label>
                <ExpressionAutocomplete
                  value={item.value ?? ""}
                  onChange={(v) => updateItem(i, { value: v })}
                  workflow={workflowName}
                  node={selectedNodeId ?? undefined}
                  className="w-full px-2 py-1.5 text-sm font-mono border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-400 focus:border-transparent"
                  placeholder="value or {{ expression }}"
                />
              </div>
            </div>

            {/* Path + Domain */}
            <div className="grid grid-cols-2 gap-2">
              <div>
                <label className="text-[10px] text-gray-400 uppercase block mb-0.5">
                  Path
                </label>
                <input
                  type="text"
                  value={item.path ?? ""}
                  onChange={(e) => updateItem(i, { path: e.target.value })}
                  className="w-full px-2 py-1.5 text-sm font-mono border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-400 focus:border-transparent"
                  placeholder="/"
                />
              </div>
              <div>
                <label className="text-[10px] text-gray-400 uppercase block mb-0.5">
                  Domain
                </label>
                <input
                  type="text"
                  value={item.domain ?? ""}
                  onChange={(e) => updateItem(i, { domain: e.target.value })}
                  className="w-full px-2 py-1.5 text-sm font-mono border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-400 focus:border-transparent"
                  placeholder="e.g. example.com"
                />
              </div>
            </div>

            {/* Max Age + SameSite */}
            <div className="grid grid-cols-2 gap-2">
              <div>
                <label className="text-[10px] text-gray-400 uppercase block mb-0.5">
                  Max Age (seconds)
                </label>
                <input
                  type="number"
                  value={item.max_age ?? ""}
                  onChange={(e) =>
                    updateItem(i, {
                      max_age: e.target.value
                        ? Number(e.target.value)
                        : undefined,
                    })
                  }
                  className="w-full px-2 py-1.5 text-sm font-mono border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-400 focus:border-transparent"
                  placeholder="e.g. 3600"
                />
              </div>
              <div>
                <label className="text-[10px] text-gray-400 uppercase block mb-0.5">
                  SameSite
                </label>
                <select
                  value={item.same_site ?? ""}
                  onChange={(e) =>
                    updateItem(i, { same_site: e.target.value || undefined })
                  }
                  className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-400 focus:border-transparent"
                >
                  {SAME_SITE_OPTIONS.map((opt) => (
                    <option key={opt} value={opt}>
                      {opt || "(default)"}
                    </option>
                  ))}
                </select>
              </div>
            </div>

            {/* Secure + HttpOnly */}
            <div className="flex items-center gap-4">
              <label className="flex items-center gap-1.5 text-sm text-gray-600 cursor-pointer">
                <input
                  type="checkbox"
                  checked={item.secure ?? false}
                  onChange={(e) => updateItem(i, { secure: e.target.checked })}
                  className="rounded border-gray-300 text-blue-500 focus:ring-blue-400"
                />
                Secure
              </label>
              <label className="flex items-center gap-1.5 text-sm text-gray-600 cursor-pointer">
                <input
                  type="checkbox"
                  checked={item.http_only ?? false}
                  onChange={(e) =>
                    updateItem(i, { http_only: e.target.checked })
                  }
                  className="rounded border-gray-300 text-blue-500 focus:ring-blue-400"
                />
                HttpOnly
              </label>
            </div>
          </div>
        ))}
      </div>
      <button
        type="button"
        onClick={addItem}
        className="mt-1.5 text-xs text-blue-500 hover:text-blue-700"
      >
        + Add cookie
      </button>
    </div>
  );
}
