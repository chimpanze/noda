import { useState, useCallback } from "react";
import { Trash2, Plus, X } from "lucide-react";

export interface RouteConfig {
  id: string;
  method: string;
  path: string;
  summary?: string;
  tags?: string[];
  middleware?: string[];
  body?: { schema?: unknown; raw?: boolean };
  trigger?: {
    workflow: string;
    input?: Record<string, string>;
    files?: string[];
  };
  [key: string]: unknown;
}

const METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"];

interface RouteFormPanelProps {
  route: RouteConfig;
  workflows: string[];
  onChange: (route: RouteConfig) => void;
  onSave: () => void;
  onDelete?: () => void;
  saving: boolean;
  isNew?: boolean;
}

export function RouteFormPanel({
  route,
  workflows,
  onChange,
  onSave,
  onDelete,
  saving,
  isNew,
}: RouteFormPanelProps) {
  const update = useCallback(
    (patch: Partial<RouteConfig>) => onChange({ ...route, ...patch }),
    [route, onChange]
  );

  const updateTrigger = useCallback(
    (patch: Partial<NonNullable<RouteConfig["trigger"]>>) => {
      onChange({
        ...route,
        trigger: { ...route.trigger, workflow: route.trigger?.workflow ?? "", ...patch },
      });
    },
    [route, onChange]
  );

  return (
    <div className="max-w-2xl space-y-5">
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-semibold text-gray-900">
          {isNew ? "New Route" : route.id}
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
            disabled={saving || !route.id || !route.path}
            className="px-4 py-1.5 text-sm text-white bg-blue-500 rounded hover:bg-blue-600 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {saving ? "Saving..." : "Save"}
          </button>
        </div>
      </div>

      {/* ID */}
      <Field label="ID">
        <input
          type="text"
          value={route.id}
          onChange={(e) => update({ id: e.target.value })}
          className="input-field font-mono"
          placeholder="e.g. create-task"
        />
      </Field>

      {/* Method + Path */}
      <div className="grid grid-cols-[140px_1fr] gap-3">
        <Field label="Method">
          <select
            value={route.method}
            onChange={(e) => update({ method: e.target.value })}
            className="input-field"
          >
            {METHODS.map((m) => (
              <option key={m} value={m}>{m}</option>
            ))}
          </select>
        </Field>
        <Field label="Path">
          <input
            type="text"
            value={route.path}
            onChange={(e) => update({ path: e.target.value })}
            className="input-field font-mono"
            placeholder="/api/tasks/:id"
          />
        </Field>
      </div>

      {/* Summary */}
      <Field label="Summary">
        <input
          type="text"
          value={route.summary ?? ""}
          onChange={(e) => update({ summary: e.target.value || undefined })}
          className="input-field"
          placeholder="Brief description of this route"
        />
      </Field>

      {/* Tags */}
      <TagInput
        label="Tags"
        values={route.tags ?? []}
        onChange={(tags) => update({ tags: tags.length > 0 ? tags : undefined })}
        placeholder="Add tag..."
      />

      {/* Middleware */}
      <TagInput
        label="Middleware"
        values={route.middleware ?? []}
        onChange={(middleware) => update({ middleware: middleware.length > 0 ? middleware : undefined })}
        placeholder="e.g. auth.jwt"
      />

      {/* Trigger */}
      <div className="border-t border-gray-200 pt-4">
        <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-3">
          Trigger
        </h4>

        <Field label="Workflow">
          <select
            value={route.trigger?.workflow ?? ""}
            onChange={(e) => updateTrigger({ workflow: e.target.value })}
            className="input-field"
          >
            <option value="">Select workflow...</option>
            {workflows.map((wf) => (
              <option key={wf} value={extractWorkflowName(wf)}>
                {extractWorkflowName(wf)}
              </option>
            ))}
          </select>
        </Field>

        {/* Input Mapping */}
        <KeyValueEditor
          label="Input Mapping"
          entries={route.trigger?.input ?? {}}
          onChange={(input) =>
            updateTrigger({ input: Object.keys(input).length > 0 ? input : undefined })
          }
          valuePlaceholder="{{ body.field }}"
        />

        {/* Files */}
        <TagInput
          label="File Fields"
          values={route.trigger?.files ?? []}
          onChange={(files) =>
            updateTrigger({ files: files.length > 0 ? files : undefined })
          }
          placeholder="e.g. file"
        />
      </div>

      {/* Body Schema */}
      <div className="border-t border-gray-200 pt-4">
        <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-3">
          Request Body
        </h4>
        <Field label="Schema $ref">
          <input
            type="text"
            value={
              route.body?.schema &&
              typeof route.body.schema === "object" &&
              (route.body.schema as Record<string, unknown>)["$ref"]
                ? String((route.body.schema as Record<string, unknown>)["$ref"])
                : ""
            }
            onChange={(e) => {
              const ref = e.target.value;
              if (ref) {
                update({ body: { ...route.body, schema: { $ref: ref } } });
              } else {
                const { schema: _, ...rest } = route.body ?? {};
                update({ body: Object.keys(rest).length > 0 ? rest : undefined });
              }
            }}
            className="input-field font-mono"
            placeholder="schemas/CreateTask"
          />
        </Field>
      </div>

      {/* Raw JSON preview */}
      <div className="border-t border-gray-200 pt-4">
        <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-2">
          JSON Preview
        </h4>
        <pre className="p-3 bg-gray-50 rounded text-xs text-gray-600 overflow-x-auto whitespace-pre-wrap border border-gray-200">
          {JSON.stringify(route, null, 2)}
        </pre>
      </div>
    </div>
  );
}

// --- Shared sub-components ---

function Field({ label, children }: { label: string; children: React.ReactNode }) {
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
    <Field label={label}>
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
    </Field>
  );
}

function KeyValueEditor({
  label,
  entries,
  onChange,
  valuePlaceholder,
}: {
  label: string;
  entries: Record<string, string>;
  onChange: (entries: Record<string, string>) => void;
  valuePlaceholder?: string;
}) {
  const pairs = Object.entries(entries);

  const updateKey = (oldKey: string, newKey: string) => {
    const next: Record<string, string> = {};
    for (const [k, v] of pairs) {
      next[k === oldKey ? newKey : k] = v;
    }
    onChange(next);
  };

  const updateValue = (key: string, value: string) => {
    onChange({ ...entries, [key]: value });
  };

  const remove = (key: string) => {
    const next = { ...entries };
    delete next[key];
    onChange(next);
  };

  const add = () => {
    let newKey = "key";
    let i = 1;
    while (newKey in entries) newKey = `key${i++}`;
    onChange({ ...entries, [newKey]: "" });
  };

  return (
    <div className="mt-3">
      <label className="text-xs font-medium text-gray-400 uppercase block mb-1">
        {label}
      </label>
      <div className="space-y-1">
        {pairs.map(([key, val]) => (
          <div key={key} className="flex items-center gap-1">
            <input
              type="text"
              value={key}
              onChange={(e) => updateKey(key, e.target.value)}
              className="w-1/3 input-field font-mono"
              placeholder="key"
            />
            <span className="text-gray-400 text-xs">:</span>
            <input
              type="text"
              value={val}
              onChange={(e) => updateValue(key, e.target.value)}
              className="flex-1 input-field font-mono"
              placeholder={valuePlaceholder}
            />
            <button
              type="button"
              onClick={() => remove(key)}
              className="px-1 text-red-400 hover:text-red-600"
            >
              <X size={14} />
            </button>
          </div>
        ))}
      </div>
      <button
        type="button"
        onClick={add}
        className="mt-1 text-xs text-blue-500 hover:text-blue-700"
      >
        + Add field
      </button>
    </div>
  );
}

function extractWorkflowName(path: string): string {
  return path.replace(/^workflows\//, "").replace(/\.json$/, "");
}
