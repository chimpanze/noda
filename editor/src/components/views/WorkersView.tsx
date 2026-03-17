import { useEffect, useState, useCallback } from "react";
import { ExternalLink, X } from "lucide-react";
import { ConfigListDetail } from "@/components/ui/ConfigListDetail";
import { VarPickerButton } from "@/components/widgets/VarPickerButton";
import { ViewHeader } from "@/components/layout/ViewHeader";
import { Field } from "@/components/ui/Field";
import { DetailHeader } from "@/components/ui/DetailHeader";
import { KeyValueEditor } from "@/components/ui/KeyValueEditor";
import * as api from "@/api/client";
import { useEditorStore } from "@/stores/editor";
import { showToast } from "@/utils/toast";

interface WorkerConfig {
  id: string;
  services?: { stream?: string };
  subscribe?: { topic?: string; group?: string };
  concurrency?: number;
  timeout?: string;
  middleware?: string[];
  dead_letter?: { topic?: string; after?: number };
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
  const [middlewareNames, setMiddlewareNames] = useState<string[]>([]);
  const [serviceNames, setServiceNames] = useState<string[]>([]);

  const reload = useCallback(async () => {
    if (!files?.workers) return;
    setLoading(true);
    try {
      const [, mwInfo, services] = await Promise.all([
        (async () => {
          const results: WorkerEntry[] = [];
          await Promise.all(
            files.workers!.map(async (path) => {
              const data = (await api.readFile(path)) as WorkerConfig;
              results.push({ filePath: path, worker: data });
            }),
          );
          setEntries(results);
        })(),
        api.listMiddleware(),
        api.listServices(),
      ]);
      const instanceNames = Object.keys(mwInfo.instances ?? {});
      setMiddlewareNames([
        ...mwInfo.middleware.map((m) => m.name),
        ...instanceNames,
      ]);
      setServiceNames(services.map((s) => s.name));
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
    [entries],
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
      if (!clean.services?.stream) delete clean.services;
      if (!clean.timeout) delete clean.timeout;
      if (clean.dead_letter && !clean.dead_letter.topic)
        delete clean.dead_letter;
      if (clean.trigger) {
        if (
          !clean.trigger.input ||
          Object.keys(clean.trigger.input).length === 0
        )
          delete clean.trigger.input;
      }

      const filePath = isNew
        ? `workers/${clean.id}.json`
        : entries[selectedIndex!].filePath;
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
    [files?.workflows, setActiveView, setActiveWorkflow],
  );

  const update = useCallback(
    (patch: Partial<WorkerConfig>) => {
      if (editWorker) setEditWorker({ ...editWorker, ...patch });
    },
    [editWorker],
  );

  const updateTrigger = useCallback(
    (patch: Partial<NonNullable<WorkerConfig["trigger"]>>) => {
      if (!editWorker) return;
      setEditWorker({
        ...editWorker,
        trigger: {
          ...editWorker.trigger,
          workflow: editWorker.trigger?.workflow ?? "",
          ...patch,
        },
      });
    },
    [editWorker],
  );

  if (loading) {
    return <div className="p-6 text-sm text-gray-400">Loading workers...</div>;
  }

  return (
    <div className="flex-1 flex flex-col min-h-0">
      <ViewHeader
        title="Workers"
        subtitle="Background workers subscribing to streams and topics"
      />
      <ConfigListDetail
        items={entries}
        getKey={(e) => e.filePath}
        selectedKey={
          isNew ? null : (selectedIndex !== null ? entries[selectedIndex]?.filePath ?? null : null)
        }
        onSelect={(key) => {
          const idx = entries.findIndex((e) => e.filePath === key);
          if (idx >= 0) selectWorker(idx);
        }}
        renderItem={(entry) => (
          <>
            <div className="text-sm font-medium text-gray-800 truncate">
              {entry.worker.id}
            </div>
            <div className="text-xs text-gray-400 truncate">
              {entry.worker.subscribe?.topic ?? "—"} &middot; concurrency{" "}
              {entry.worker.concurrency ?? 1}
            </div>
          </>
        )}
        title={`Workers (${entries.length})`}
        onNew={startNew}
        emptyMessage="No workers configured."
      >
        {editWorker ? (
            <div className="max-w-2xl space-y-5">
              <DetailHeader
                title={isNew ? "New Worker" : editWorker.id}
                isNew={isNew}
                saving={saving}
                onSave={handleSave}
                onDelete={handleDelete}
                saveDisabled={!editWorker.id}
              />

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
                <select
                  value={editWorker.services?.stream ?? ""}
                  onChange={(e) =>
                    update({ services: { stream: e.target.value } })
                  }
                  className="input-field"
                >
                  <option value="">Select service...</option>
                  {serviceNames.map((n) => (
                    <option key={n} value={n}>
                      {n}
                    </option>
                  ))}
                </select>
              </Field>

              {/* Subscribe */}
              <div className="grid grid-cols-2 gap-3">
                <Field label="Topic">
                  <div className="flex items-center gap-1">
                    <input
                      type="text"
                      value={editWorker.subscribe?.topic ?? ""}
                      onChange={(e) =>
                        update({
                          subscribe: {
                            ...editWorker.subscribe,
                            topic: e.target.value,
                          },
                        })
                      }
                      className="input-field font-mono flex-1"
                      placeholder="e.g. task.created"
                    />
                    <VarPickerButton
                      onSelect={(ref) =>
                        update({
                          subscribe: { ...editWorker.subscribe, topic: ref },
                        })
                      }
                      currentValue={editWorker.subscribe?.topic}
                    />
                  </div>
                </Field>
                <Field label="Consumer Group">
                  <div className="flex items-center gap-1">
                    <input
                      type="text"
                      value={editWorker.subscribe?.group ?? ""}
                      onChange={(e) =>
                        update({
                          subscribe: {
                            ...editWorker.subscribe,
                            group: e.target.value,
                          },
                        })
                      }
                      className="input-field flex-1"
                      placeholder="e.g. my-workers"
                    />
                    <VarPickerButton
                      onSelect={(ref) =>
                        update({
                          subscribe: { ...editWorker.subscribe, group: ref },
                        })
                      }
                      currentValue={editWorker.subscribe?.group}
                    />
                  </div>
                </Field>
              </div>

              {/* Concurrency & Timeout */}
              <div className="grid grid-cols-2 gap-3">
                <Field label="Concurrency">
                  <input
                    type="number"
                    min={1}
                    step={1}
                    value={editWorker.concurrency ?? 1}
                    onChange={(e) =>
                      update({ concurrency: parseInt(e.target.value, 10) || 1 })
                    }
                    className="input-field"
                  />
                </Field>
                <Field label="Timeout">
                  <input
                    type="text"
                    value={editWorker.timeout ?? ""}
                    onChange={(e) => update({ timeout: e.target.value })}
                    className="input-field font-mono"
                    placeholder="5m"
                  />
                </Field>
              </div>

              {/* Middleware */}
              <Field label="Middleware">
                <div className="flex flex-wrap gap-1.5 mb-1.5">
                  {(editWorker.middleware ?? []).map((mw) => (
                    <span
                      key={mw}
                      className="inline-flex items-center gap-1 px-2 py-0.5 text-xs bg-gray-100 text-gray-700 rounded"
                    >
                      {mw}
                      <button
                        type="button"
                        onClick={() =>
                          update({
                            middleware: (editWorker.middleware ?? []).filter(
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
                    if (val && !(editWorker.middleware ?? []).includes(val)) {
                      update({
                        middleware: [...(editWorker.middleware ?? []), val],
                      });
                    }
                  }}
                  className="input-field"
                >
                  <option value="">Add middleware...</option>
                  {middlewareNames
                    .filter((n) => !(editWorker.middleware ?? []).includes(n))
                    .map((n) => (
                      <option key={n} value={n}>
                        {n}
                      </option>
                    ))}
                </select>
              </Field>

              {/* Dead Letter */}
              <div className="border-t border-gray-200 pt-4">
                <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-3">
                  Dead Letter Queue
                </h4>
                <div className="grid grid-cols-2 gap-3">
                  <Field label="Topic">
                    <div className="flex items-center gap-1">
                      <input
                        type="text"
                        value={editWorker.dead_letter?.topic ?? ""}
                        onChange={(e) =>
                          update({
                            dead_letter: {
                              ...editWorker.dead_letter,
                              topic: e.target.value || undefined,
                            },
                          })
                        }
                        className="input-field font-mono flex-1"
                        placeholder="e.g. task.failed"
                      />
                      <VarPickerButton
                        onSelect={(ref) =>
                          update({
                            dead_letter: {
                              ...editWorker.dead_letter,
                              topic: ref,
                            },
                          })
                        }
                        currentValue={editWorker.dead_letter?.topic}
                      />
                    </div>
                  </Field>
                  <Field label="After (attempts)">
                    <input
                      type="number"
                      min={1}
                      value={editWorker.dead_letter?.after ?? ""}
                      onChange={(e) => {
                        const val = parseInt(e.target.value, 10);
                        update({
                          dead_letter: {
                            ...editWorker.dead_letter,
                            after: isNaN(val) ? undefined : val,
                          },
                        });
                      }}
                      className="input-field"
                      placeholder="3"
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
                      onChange={(e) =>
                        updateTrigger({ workflow: e.target.value })
                      }
                      className="input-field flex-1"
                    >
                      <option value="">Select workflow...</option>
                      {(files?.workflows ?? []).map((wf) => {
                        const name = wf
                          .replace(/^workflows\//, "")
                          .replace(/\.json$/, "");
                        return (
                          <option key={wf} value={name}>
                            {name}
                          </option>
                        );
                      })}
                    </select>
                    {editWorker.trigger?.workflow && (
                      <button
                        onClick={() =>
                          goToWorkflow(editWorker.trigger!.workflow)
                        }
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
                      updateTrigger({
                        input:
                          Object.keys(input).length > 0 ? input : undefined,
                      })
                    }
                    workflow={editWorker.trigger?.workflow}
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
      </ConfigListDetail>
    </div>
  );
}

