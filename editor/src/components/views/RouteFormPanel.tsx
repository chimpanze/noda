import { useState, useCallback } from "react";
import { Trash2, Plus, X } from "lucide-react";
import type { SchemaInfo } from "@/types";
import { ExpressionAutocomplete } from "@/components/widgets/ExpressionAutocomplete";
import { SchemaSelect } from "@/components/widgets/SchemaSelect";

export interface RouteConfig {
  id: string;
  method: string;
  path: string;
  summary?: string;
  tags?: string[];
  middleware?: string[];
  middleware_preset?: string;
  params?: { schema?: unknown };
  query?: { schema?: unknown };
  body?: { schema?: unknown; raw?: boolean; validate?: boolean; content_type?: string };
  response?: {
    validate?: string;
    statuses?: Record<string, { description?: string; schema?: { $ref: string } }>;
  };
  response_timeout?: string;
  trigger?: {
    workflow: string;
    input?: Record<string, string>;
    files?: string[];
    raw_body?: boolean;
  };
  [key: string]: unknown;
}

const METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"];
const STATUS_CODES = ["200", "201", "204", "400", "401", "403", "404", "409", "422", "500"];

interface RouteFormPanelProps {
  route: RouteConfig;
  workflows: string[];
  middlewareNames: string[];
  middlewarePresets: Record<string, string[]>;
  schemas: SchemaInfo[];
  onChange: (route: RouteConfig) => void;
  onSave: () => void;
  onDelete?: () => void;
  saving: boolean;
  isNew?: boolean;
}

export function RouteFormPanel({
  route,
  workflows,
  middlewareNames,
  middlewarePresets,
  schemas,
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

  const schemaRef = (obj?: { schema?: unknown }) => {
    if (!obj?.schema || typeof obj.schema !== "object") return "";
    const ref = (obj.schema as Record<string, unknown>)["$ref"];
    return ref ? String(ref) : "";
  };

  const currentBodyRef = schemaRef(route.body);
  const currentParamsRef = schemaRef(route.params);
  const currentQueryRef = schemaRef(route.query);

  const responseStatuses = route.response?.statuses ?? {};
  const responseValidate = route.response?.validate ?? "off";

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

      {/* Response Timeout */}
      <Field label="Response Timeout">
        <input
          type="text"
          value={route.response_timeout ?? ""}
          onChange={(e) => update({ response_timeout: e.target.value || undefined })}
          className="input-field"
          placeholder="e.g. 30s"
        />
      </Field>

      {/* Path Parameters Schema */}
      <Field label="Path Parameters Schema">
        <SchemaSelect
          schemas={schemas}
          value={currentParamsRef}
          onChange={(ref) => {
            if (ref) {
              update({ params: { schema: { $ref: ref } } });
            } else {
              update({ params: undefined });
            }
          }}
          className="input-field font-mono"
        />
      </Field>

      {/* Query Parameters Schema */}
      <Field label="Query Parameters Schema">
        <SchemaSelect
          schemas={schemas}
          value={currentQueryRef}
          onChange={(ref) => {
            if (ref) {
              update({ query: { schema: { $ref: ref } } });
            } else {
              update({ query: undefined });
            }
          }}
          className="input-field font-mono"
        />
      </Field>

      {/* Tags */}
      <TagInput
        label="Tags"
        values={route.tags ?? []}
        onChange={(tags) => update({ tags: tags.length > 0 ? tags : undefined })}
        placeholder="Add tag..."
      />

      {/* Middleware Preset */}
      <Field label="Middleware Preset">
        <select
          value={route.middleware_preset ?? ""}
          onChange={(e) => update({ middleware_preset: e.target.value || undefined })}
          className="input-field"
        >
          <option value="">None</option>
          {Object.entries(middlewarePresets).map(([name, mws]) => (
            <option key={name} value={name}>
              {name} ({mws.join(", ")})
            </option>
          ))}
        </select>
      </Field>

      {/* Middleware */}
      <Field label="Middleware">
        <div className="flex flex-wrap gap-1.5 mb-1.5">
          {(route.middleware ?? []).map((mw) => (
            <span
              key={mw}
              className="inline-flex items-center gap-1 px-2 py-0.5 text-xs bg-gray-100 text-gray-700 rounded"
            >
              {mw}
              <button
                type="button"
                onClick={() =>
                  update({ middleware: (route.middleware ?? []).filter((x) => x !== mw) })
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
            if (val && !(route.middleware ?? []).includes(val)) {
              update({ middleware: [...(route.middleware ?? []), val] });
            }
          }}
          className="input-field"
        >
          <option value="">Add middleware...</option>
          {middlewareNames
            .filter((n) => !(route.middleware ?? []).includes(n))
            .map((n) => (
              <option key={n} value={n}>{n}</option>
            ))}
        </select>
      </Field>

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
          workflow={route.trigger?.workflow}
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

        {/* Raw Body */}
        <div className="mt-2">
          <label className="inline-flex items-center gap-2 text-sm text-gray-600 cursor-pointer">
            <input
              type="checkbox"
              checked={route.trigger?.raw_body ?? false}
              onChange={(e) => updateTrigger({ raw_body: e.target.checked || undefined })}
              className="rounded border-gray-300"
            />
            Pass raw body
          </label>
        </div>
      </div>

      {/* Body Schema */}
      <div className="border-t border-gray-200 pt-4">
        <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-3">
          Request Body
        </h4>
        <Field label="Content Type">
          <select
            value={route.body?.content_type ?? ""}
            onChange={(e) => {
              const ct = e.target.value;
              if (ct) {
                update({ body: { ...route.body, content_type: ct } });
              } else {
                if (route.body) {
                  const { content_type: _, ...rest } = route.body;
                  update({ body: Object.keys(rest).length > 0 ? rest : undefined });
                }
              }
            }}
            className="input-field"
          >
            <option value="">Default (application/json)</option>
            <option value="application/json">application/json</option>
            <option value="multipart/form-data">multipart/form-data</option>
            <option value="application/x-www-form-urlencoded">application/x-www-form-urlencoded</option>
            <option value="text/plain">text/plain</option>
          </select>
        </Field>
        <Field label="Schema">
          <SchemaSelect
            schemas={schemas}
            value={currentBodyRef}
            onChange={(ref) => {
              if (ref) {
                update({ body: { ...route.body, schema: { $ref: ref } } });
              } else {
                const { schema: _, ...rest } = route.body ?? {};
                update({ body: Object.keys(rest).length > 0 ? rest : undefined });
              }
            }}
            className="input-field font-mono"
          />
        </Field>
        <div className="mt-2">
          <label className="inline-flex items-center gap-2 text-sm text-gray-600 cursor-pointer">
            <input
              type="checkbox"
              checked={route.body?.validate !== false}
              onChange={(e) => {
                const validate = e.target.checked;
                if (validate) {
                  // true/undefined is default — remove explicit flag
                  const { validate: _, ...rest } = route.body ?? {};
                  update({ body: Object.keys(rest).length > 0 ? rest : undefined });
                } else {
                  update({ body: { ...route.body, validate: false } });
                }
              }}
              className="rounded border-gray-300"
            />
            Validate request body
          </label>
        </div>
      </div>

      {/* Response Validation */}
      <div className="border-t border-gray-200 pt-4">
        <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-3">
          Response Validation
        </h4>

        <Field label="Validate Mode">
          <select
            value={responseValidate}
            onChange={(e) => {
              const mode = e.target.value;
              if (mode === "off") {
                const { validate: _, ...rest } = route.response ?? {};
                update({ response: Object.keys(rest).length > 0 ? rest : undefined });
              } else {
                update({ response: { ...route.response, validate: mode } });
              }
            }}
            className="input-field"
          >
            <option value="off">Off</option>
            <option value="warn">Warn</option>
            <option value="strict">Strict</option>
          </select>
        </Field>

        <div className="mt-3 space-y-2">
          {Object.entries(responseStatuses).map(([code, entry]) => (
            <div key={code} className="flex items-start gap-2 p-2 bg-gray-50 rounded border border-gray-200">
              <div className="shrink-0">
                <label className="text-[10px] text-gray-400 uppercase block">Status</label>
                <span className="text-sm font-mono font-medium text-gray-700">{code}</span>
              </div>
              <div className="flex-1 min-w-0">
                <label className="text-[10px] text-gray-400 uppercase block">Description</label>
                <input
                  type="text"
                  value={entry.description ?? ""}
                  onChange={(e) => {
                    const statuses = { ...responseStatuses };
                    statuses[code] = { ...statuses[code], description: e.target.value || undefined };
                    update({ response: { ...route.response, statuses } });
                  }}
                  className="input-field text-xs"
                  placeholder="Description"
                />
              </div>
              <div className="flex-1 min-w-0">
                <label className="text-[10px] text-gray-400 uppercase block">Schema</label>
                <SchemaSelect
                  schemas={schemas}
                  value={entry.schema?.$ref ?? ""}
                  onChange={(ref) => {
                    const statuses = { ...responseStatuses };
                    statuses[code] = {
                      ...statuses[code],
                      schema: ref ? { $ref: ref } : undefined,
                    };
                    update({ response: { ...route.response, statuses } });
                  }}
                  className="input-field text-xs font-mono"
                />
              </div>
              <button
                type="button"
                onClick={() => {
                  const statuses = { ...responseStatuses };
                  delete statuses[code];
                  update({
                    response: {
                      ...route.response,
                      statuses: Object.keys(statuses).length > 0 ? statuses : undefined,
                    },
                  });
                }}
                className="mt-3 px-1 text-red-400 hover:text-red-600 shrink-0"
              >
                <X size={14} />
              </button>
            </div>
          ))}
        </div>

        <AddStatusButton
          existingCodes={Object.keys(responseStatuses)}
          onAdd={(code) => {
            const statuses = { ...responseStatuses, [code]: {} };
            update({ response: { ...route.response, statuses } });
          }}
        />
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
  workflow,
}: {
  label: string;
  entries: Record<string, string>;
  onChange: (entries: Record<string, string>) => void;
  valuePlaceholder?: string;
  workflow?: string;
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
              className="shrink-0 input-field !w-1/3 font-mono"
              placeholder="key"
            />
            <span className="text-gray-400 text-xs">:</span>
            {workflow ? (
              <div className="flex-1 min-w-0">
                <ExpressionAutocomplete
                  value={val}
                  onChange={(v) => updateValue(key, v)}
                  workflow={workflow}
                  className="input-field !w-auto font-mono"
                  placeholder={valuePlaceholder}
                />
              </div>
            ) : (
              <input
                type="text"
                value={val}
                onChange={(e) => updateValue(key, e.target.value)}
                className="flex-1 min-w-0 input-field !w-auto font-mono"
                placeholder={valuePlaceholder}
              />
            )}
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

function AddStatusButton({
  existingCodes,
  onAdd,
}: {
  existingCodes: string[];
  onAdd: (code: string) => void;
}) {
  return (
    <div className="mt-2">
      <select
        value=""
        onChange={(e) => {
          if (e.target.value) onAdd(e.target.value);
        }}
        className="text-xs text-blue-500 bg-transparent border-none cursor-pointer hover:text-blue-700 p-0"
      >
        <option value="">+ Add Status Code</option>
        {STATUS_CODES.filter((c) => !existingCodes.includes(c)).map((c) => (
          <option key={c} value={c}>{c}</option>
        ))}
      </select>
    </div>
  );
}

function extractWorkflowName(path: string): string {
  return path.replace(/^workflows\//, "").replace(/\.json$/, "");
}
