import { useState } from "react";
import { Trash2, X, Plus } from "lucide-react";
import type { RouteGroupConfig } from "@/types";

interface RouteGroupFormPanelProps {
  prefix: string;
  group: RouteGroupConfig;
  middlewareNames: string[];
  middlewarePresets: Record<string, string[]>;
  onChange: (group: RouteGroupConfig) => void;
  onSave: () => void;
  onDelete?: () => void;
  saving: boolean;
  isNew: boolean;
}

export function RouteGroupFormPanel({
  prefix,
  group,
  middlewareNames,
  middlewarePresets,
  onChange,
  onSave,
  onDelete,
  saving,
  isNew,
}: RouteGroupFormPanelProps) {
  return (
    <div className="max-w-2xl space-y-5">
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-semibold text-gray-900">
          {isNew ? "New Route Group" : `Group: ${prefix}`}
        </h3>
        <div className="flex items-center gap-2">
          {onDelete && (
            <button
              onClick={onDelete}
              className="px-3 py-1.5 text-sm text-red-600 border border-red-300 rounded hover:bg-red-50"
            >
              <Trash2 size={14} className="inline mr-1" />
              Delete
            </button>
          )}
          <button
            onClick={onSave}
            disabled={saving}
            className="px-4 py-1.5 text-sm text-white bg-blue-500 rounded hover:bg-blue-600 disabled:opacity-50"
          >
            {saving ? "Saving..." : "Save"}
          </button>
        </div>
      </div>

      {/* Path Prefix */}
      <FieldLabel label="Path Prefix">
        <div className="input-field bg-gray-50 text-gray-600 font-mono">
          {prefix}
        </div>
      </FieldLabel>

      {/* Middleware Preset */}
      <FieldLabel label="Middleware Preset">
        <select
          value={group.middleware_preset ?? ""}
          onChange={(e) =>
            onChange({
              ...group,
              middleware_preset: e.target.value || undefined,
            })
          }
          className="input-field"
        >
          <option value="">None</option>
          {Object.entries(middlewarePresets).map(([name, mws]) => (
            <option key={name} value={name}>
              {name} ({mws.join(", ")})
            </option>
          ))}
        </select>
      </FieldLabel>

      {/* Middleware */}
      <FieldLabel label="Middleware">
        <div className="flex flex-wrap gap-1.5 mb-1.5">
          {(group.middleware ?? []).map((mw) => (
            <span
              key={mw}
              className="inline-flex items-center gap-1 px-2 py-0.5 text-xs bg-gray-100 text-gray-700 rounded"
            >
              {mw}
              <button
                type="button"
                onClick={() =>
                  onChange({
                    ...group,
                    middleware: (group.middleware ?? []).filter(
                      (x) => x !== mw,
                    ),
                  })
                }
                className="text-gray-400 hover:text-gray-600"
              >
                <X size={10} />
              </button>
            </span>
          ))}
        </div>
        <select
          value=""
          onChange={(e) => {
            const val = e.target.value;
            if (val && !(group.middleware ?? []).includes(val)) {
              onChange({
                ...group,
                middleware: [...(group.middleware ?? []), val],
              });
            }
          }}
          className="input-field"
        >
          <option value="">Add middleware...</option>
          {middlewareNames
            .filter((n) => !(group.middleware ?? []).includes(n))
            .map((n) => (
              <option key={n} value={n}>
                {n}
              </option>
            ))}
        </select>
      </FieldLabel>

      {/* Tags */}
      <TagInput
        label="Tags"
        values={group.tags ?? []}
        onChange={(tags) =>
          onChange({ ...group, tags: tags.length > 0 ? tags : undefined })
        }
        placeholder="Add tag..."
      />
    </div>
  );
}

function FieldLabel({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div>
      <label className="text-xs font-medium text-gray-400 uppercase block mb-1">
        {label}
      </label>
      {children}
    </div>
  );
}

function TagInput({
  label,
  values,
  onChange,
  placeholder,
}: {
  label: string;
  values: string[];
  onChange: (values: string[]) => void;
  placeholder?: string;
}) {
  const [draft, setDraft] = useState("");

  const add = () => {
    const v = draft.trim();
    if (v && !values.includes(v)) {
      onChange([...values, v]);
    }
    setDraft("");
  };

  return (
    <FieldLabel label={label}>
      <div className="flex flex-wrap gap-1.5 mb-1.5">
        {values.map((v) => (
          <span
            key={v}
            className="inline-flex items-center gap-1 px-2 py-0.5 text-xs bg-gray-100 text-gray-700 rounded"
          >
            {v}
            <button
              type="button"
              onClick={() => onChange(values.filter((x) => x !== v))}
              className="text-gray-400 hover:text-gray-600"
            >
              <X size={10} />
            </button>
          </span>
        ))}
      </div>
      <div className="flex gap-1">
        <input
          type="text"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              add();
            }
          }}
          className="input-field flex-1"
          placeholder={placeholder}
        />
        <button
          type="button"
          onClick={add}
          disabled={!draft.trim()}
          className="px-2 py-1.5 text-sm text-blue-500 hover:text-blue-700 disabled:opacity-30"
        >
          <Plus size={14} />
        </button>
      </div>
    </FieldLabel>
  );
}
