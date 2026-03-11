import { useEffect, useState, useCallback } from "react";
import { Plus, Trash2, ExternalLink } from "lucide-react";
import * as api from "@/api/client";
import { useEditorStore } from "@/stores/editor";
import { showToast } from "@/components/panels/Toast";

interface WorkerConfig {
  id: string;
  services?: Record<string, string>;
  subscribe?: { topic?: string; group?: string };
  concurrency?: number;
  middleware?: string[];
  retry?: { max_attempts?: number; dlq?: string };
  trigger?: {
    workflow: string;
    input?: Record<string, string>;
  };
  [key: string]: unknown;
}

interface WorkerEntry {
  filePath: string;
  worker: WorkerConfig;
}

export function WorkersView() {
  const files = useEditorStore((s) => s.files);
  const loadFiles = useEditorStore((s) => s.loadFiles);
  const setActiveView = useEditorStore((s) => s.setActiveView);
  const setActiveWorkflow = useEditorStore((s) => s.setActiveWorkflow);

  const [entries, setEntries] = useState<WorkerEntry[]>([]);
  const [selectedIndex, setSelectedIndex] = useState<number | null>(null);
  const [editWorker, setEditWorker] = useState<WorkerConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [isNew, setIsNew] = useState(false);

  const reload = useCallback(async () => {
    if (!files?.workers) return;
    setLoading(true);
    try {
      const results: WorkerEntry[] = [];
      await Promise.all(
        files.workers.map(async (path) => {
          const data = (await api.readFile(path)) as WorkerConfig;
          results.push({ filePath: path, worker: data });
        })
      );
      setEntries(results);
    } finally {
      setLoading(false);
    }
  }, [files?.workers]);

  useEffect(() => {
    reload();
  }, [reload]);

  const selectWorker = useCallback(
    (index: number) => {
      setSelectedIndex(index);
      setEditWorker(structuredClone(entries[index].worker));
      setIsNew(false);
    },
    [entries]
  );

  const startNew = useCallback(() => {
    setSelectedIndex(null);
    setIsNew(true);
    setEditWorker({
      id: "",
      services: { stream: "" },
      subscribe: { topic: "", group: "" },
      concurrency: 1,
      trigger: { workflow: "" },
    });
  }, []);

  const handleSave = useCallback(async () => {
    if (!editWorker?.id) return;
    setSaving(true);
    try {
      const clean = structuredClone(editWorker);
      if (!clean.middleware?.length) delete clean.middleware;
      if (clean.services && !Object.values(clean.services).some(Boolean))
        delete clean.services;
      if (clean.retry && !clean.retry.max_attempts) delete clean.retry;
      if (clean.trigger) {
        if (!clean.trigger.input || Object.keys(clean.trigger.input).length === 0)
          delete clean.trigger.input;
      }

      const filePath = isNew ? `workers/${clean.id}.json` : entries[selectedIndex!].filePath;
      await api.writeFile(filePath, clean);
      showToast({ type: "success", message: `Worker "${clean.id}" saved` });
      setIsNew(false);
      await loadFiles();
      await reload();
    } catch (err) {
      showToast({ type: "error", message: `Failed to save: ${err}` });
    } finally {
      setSaving(false);
    }
  }, [editWorker, isNew, selectedIndex, entries, loadFiles, reload]);

  const handleDelete = useCallback(async () => {
    if (selectedIndex === null) return;
    const entry = entries[selectedIndex];
    if (!confirm(`Delete worker "${entry.worker.id}"?`)) return;
    try {
      await api.deleteFile(entry.filePath);
      showToast({ type: "success", message: `Worker deleted` });
      setSelectedIndex(null);
      setEditWorker(null);
      await loadFiles();
      await reload();
    } catch (err) {
      showToast({ type: "error", message: `Failed to delete: ${err}` });
    }
  }, [selectedIndex, entries, loadFiles, reload]);

  const goToWorkflow = useCallback(
    (workflowId: string) => {
      const wfFiles = files?.workflows ?? [];
      const match = wfFiles.find((f) => f.includes(workflowId));
      if (match) {
        setActiveView("workflows");
        setActiveWorkflow(match);
      }
    },
    [files?.workflows, setActiveView, setActiveWorkflow]
  );

  const update = useCallback(
    (patch: Partial<WorkerConfig>) => {
      if (editWorker) setEditWorker({ ...editWorker, ...patch });
    },
    [editWorker]
  );

  const updateTrigger = useCallback(
    (patch: Partial<NonNullable<WorkerConfig["trigger"]>>) => {
      if (!editWorker) return;
      setEditWorker({
        ...editWorker,
        trigger: { ...editWorker.trigger, workflow: editWorker.trigger?.workflow ?? "", ...patch },
      });
    },
    [editWorker]
  );

  if (loading) {
    return <div className="p-6 text-sm text-gray-400">Loading workers...</div>;
  }

  return (
    <div className="flex-1 flex min-h-0">
      {/* Worker list */}
      <div className="w-80 border-r border-gray-200 overflow-y-auto">
        <div className="px-4 py-3 border-b border-gray-200 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-gray-800">
            Workers ({entries.length})
          </h2>
          <button
            onClick={startNew}
            className="flex items-center gap-1 px-2 py-1 text-xs text-blue-600 hover:bg-blue-50 rounded"
          >
            <Plus size={14} />
            New
          </button>
        </div>
        <div className="divide-y divide-gray-100">
          {entries.map((entry, index) => (
            <button
              key={entry.filePath}
              onClick={() => selectWorker(index)}
              className={`w-full text-left px-4 py-2.5 hover:bg-gray-50 ${
                selectedIndex === index && !isNew ? "bg-blue-50" : ""
              }`}
            >
              <div className="text-sm font-medium text-gray-800 truncate">
                {entry.worker.id}
              </div>
              <div className="text-xs text-gray-400 truncate">
                {entry.worker.subscribe?.topic ?? "—"} &middot; concurrency {entry.worker.concurrency ?? 1}
              </div>
            </button>
          ))}
          {entries.length === 0 && (
            <div className="p-4 text-sm text-gray-400">No workers configured.</div>
          )}
        </div>
      </div>

      {/* Worker editor */}
      <div className="flex-1 overflow-y-auto p-6">
        {editWorker ? (
          <div className="max-w-2xl space-y-5">
            <div className="flex items-center justify-between">
              <h3 className="text-lg font-semibold text-gray-900">
                {isNew ? "New Worker" : editWorker.id}
              </h3>
              <div className="flex items-center gap-2">
                {!isNew && (
                  <button
                    onClick={handleDelete}
                    className="px-3 py-1.5 text-sm text-red-600 border border-red-300 rounded hover:bg-red-50"
                  >
                    <Trash2 size={14} className="inline mr-1" />
                    Delete
                  </button>
                )}
                <button
                  onClick={handleSave}
                  disabled={saving || !editWorker.id}
                  className="px-4 py-1.5 text-sm text-white bg-blue-500 rounded hover:bg-blue-600 disabled:opacity-50"
                >
                  {saving ? "Saving..." : "Save"}
                </button>
              </div>
            </div>

            {/* ID */}
            <Field label="ID">
              <input
                type="text"
                value={editWorker.id}
                onChange={(e) => update({ id: e.target.value })}
                className="input-field font-mono"
                placeholder="e.g. send-notification-worker"
              />
            </Field>

            {/* Stream Service */}
            <Field label="Stream Service">
              <input
                type="text"
                value={editWorker.services?.stream ?? ""}
                onChange={(e) =>
                  update({ services: { ...editWorker.services, stream: e.target.value } })
                }
                className="input-field"
                placeholder="e.g. main-stream"
              />
            </Field>

            {/* Subscribe */}
            <div className="grid grid-cols-2 gap-3">
              <Field label="Topic">
                <input
                  type="text"
                  value={editWorker.subscribe?.topic ?? ""}
                  onChange={(e) =>
                    update({ subscribe: { ...editWorker.subscribe, topic: e.target.value } })
                  }
                  className="input-field font-mono"
                  placeholder="e.g. task.created"
                />
              </Field>
              <Field label="Consumer Group">
                <input
                  type="text"
                  value={editWorker.subscribe?.group ?? ""}
                  onChange={(e) =>
                    update({ subscribe: { ...editWorker.subscribe, group: e.target.value } })
                  }
                  className="input-field"
                  placeholder="e.g. my-workers"
                />
              </Field>
            </div>

            {/* Concurrency */}
            <Field label="Concurrency">
              <input
                type="number"
                min={1}
                step={1}
                value={editWorker.concurrency ?? 1}
                onChange={(e) => update({ concurrency: parseInt(e.target.value, 10) || 1 })}
                className="input-field w-32"
              />
            </Field>

            {/* Retry */}
            <div className="border-t border-gray-200 pt-4">
              <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-3">
                Retry
              </h4>
              <div className="grid grid-cols-2 gap-3">
                <Field label="Max Attempts">
                  <input
                    type="number"
                    min={0}
                    value={editWorker.retry?.max_attempts ?? ""}
                    onChange={(e) => {
                      const val = parseInt(e.target.value, 10);
                      update({
                        retry: {
                          ...editWorker.retry,
                          max_attempts: isNaN(val) ? undefined : val,
                        },
                      });
                    }}
                    className="input-field"
                    placeholder="0"
                  />
                </Field>
                <Field label="Dead Letter Queue">
                  <input
                    type="text"
                    value={editWorker.retry?.dlq ?? ""}
                    onChange={(e) =>
                      update({
                        retry: { ...editWorker.retry, dlq: e.target.value || undefined },
                      })
                    }
                    className="input-field"
                    placeholder="e.g. notification-dlq"
                  />
                </Field>
              </div>
            </div>

            {/* Trigger */}
            <div className="border-t border-gray-200 pt-4">
              <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-3">
                Trigger
              </h4>
              <Field label="Workflow">
                <div className="flex items-center gap-2">
                  <select
                    value={editWorker.trigger?.workflow ?? ""}
                    onChange={(e) => updateTrigger({ workflow: e.target.value })}
                    className="input-field flex-1"
                  >
                    <option value="">Select workflow...</option>
                    {(files?.workflows ?? []).map((wf) => {
                      const name = wf.replace(/^workflows\//, "").replace(/\.json$/, "");
                      return (
                        <option key={wf} value={name}>
                          {name}
                        </option>
                      );
                    })}
                  </select>
                  {editWorker.trigger?.workflow && (
                    <button
                      onClick={() => goToWorkflow(editWorker.trigger!.workflow)}
                      className="text-blue-500 hover:text-blue-700"
                      title="Open workflow"
                    >
                      <ExternalLink size={14} />
                    </button>
                  )}
                </div>
              </Field>

              {/* Input Mapping */}
              <div className="mt-3">
                <label className="text-xs font-medium text-gray-400 uppercase block mb-1">
                  Input Mapping
                </label>
                <KeyValueEditor
                  entries={editWorker.trigger?.input ?? {}}
                  onChange={(input) =>
                    updateTrigger({ input: Object.keys(input).length > 0 ? input : undefined })
                  }
                />
              </div>
            </div>

            {/* JSON Preview */}
            <div className="border-t border-gray-200 pt-4">
              <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-2">
                JSON Preview
              </h4>
              <pre className="p-3 bg-gray-50 rounded text-xs text-gray-600 overflow-x-auto whitespace-pre-wrap border border-gray-200">
                {JSON.stringify(editWorker, null, 2)}
              </pre>
            </div>
          </div>
        ) : (
          <div className="text-sm text-gray-400">
            Select a worker to edit or click "New" to create one.
          </div>
        )}
      </div>
    </div>
  );
}

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

function KeyValueEditor({
  entries,
  onChange,
}: {
  entries: Record<string, string>;
  onChange: (entries: Record<string, string>) => void;
}) {
  const pairs = Object.entries(entries);

  return (
    <>
      <div className="space-y-1">
        {pairs.map(([key, val]) => (
          <div key={key} className="flex items-center gap-1">
            <input
              type="text"
              value={key}
              onChange={(e) => {
                const next: Record<string, string> = {};
                for (const [k, v] of pairs) next[k === key ? e.target.value : k] = v;
                onChange(next);
              }}
              className="shrink-0 input-field !w-1/3 font-mono"
              placeholder="key"
            />
            <span className="text-gray-400 text-xs">:</span>
            <input
              type="text"
              value={val}
              onChange={(e) => onChange({ ...entries, [key]: e.target.value })}
              className="flex-1 min-w-0 input-field !w-auto font-mono"
              placeholder="{{ message.payload.field }}"
            />
            <button
              type="button"
              onClick={() => {
                const next = { ...entries };
                delete next[key];
                onChange(next);
              }}
              className="px-1 text-red-400 hover:text-red-600 text-sm"
            >
              &times;
            </button>
          </div>
        ))}
      </div>
      <button
        type="button"
        onClick={() => {
          let k = "key";
          let i = 1;
          while (k in entries) k = `key${i++}`;
          onChange({ ...entries, [k]: "" });
        }}
        className="mt-1 text-xs text-blue-500 hover:text-blue-700"
      >
        + Add field
      </button>
    </>
  );
}
