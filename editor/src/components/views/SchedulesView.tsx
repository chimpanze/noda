import { useEffect, useState, useCallback } from "react";
import { Plus, Trash2, ExternalLink, Clock } from "lucide-react";
import { ViewHeader } from "@/components/layout/ViewHeader";
import * as api from "@/api/client";
import { useEditorStore } from "@/stores/editor";
import { showToast } from "@/components/panels/Toast";
import { describeCron } from "@/utils/cron";

interface ScheduleConfig {
  id: string;
  cron: string;
  timezone?: string;
  services?: Record<string, string>;
  lock?: { enabled?: boolean; ttl?: string };
  trigger?: {
    workflow: string;
    input?: Record<string, string>;
  };
  [key: string]: unknown;
}

interface ScheduleEntry {
  filePath: string;
  schedule: ScheduleConfig;
}

const COMMON_TIMEZONES = [
  "UTC",
  "America/New_York",
  "America/Chicago",
  "America/Denver",
  "America/Los_Angeles",
  "Europe/London",
  "Europe/Berlin",
  "Europe/Paris",
  "Asia/Tokyo",
  "Asia/Shanghai",
  "Asia/Kolkata",
  "Australia/Sydney",
];

export function SchedulesView() {
  const files = useEditorStore((s) => s.files);
  const loadFiles = useEditorStore((s) => s.loadFiles);
  const setActiveView = useEditorStore((s) => s.setActiveView);
  const setActiveWorkflow = useEditorStore((s) => s.setActiveWorkflow);

  const [entries, setEntries] = useState<ScheduleEntry[]>([]);
  const [selectedIndex, setSelectedIndex] = useState<number | null>(null);
  const [editSchedule, setEditSchedule] = useState<ScheduleConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [isNew, setIsNew] = useState(false);
  const [serviceNames, setServiceNames] = useState<string[]>([]);

  const reload = useCallback(async () => {
    if (!files?.schedules) return;
    setLoading(true);
    try {
      const [, services] = await Promise.all([
        (async () => {
          const results: ScheduleEntry[] = [];
          await Promise.all(
            files.schedules!.map(async (path) => {
              const data = (await api.readFile(path)) as ScheduleConfig;
              results.push({ filePath: path, schedule: data });
            })
          );
          setEntries(results);
        })(),
        api.listServices(),
      ]);
      setServiceNames(services.map((s) => s.name));
    } finally {
      setLoading(false);
    }
  }, [files?.schedules]);

  useEffect(() => {
    reload();
  }, [reload]);

  const selectSchedule = useCallback(
    (index: number) => {
      setSelectedIndex(index);
      setEditSchedule(structuredClone(entries[index].schedule));
      setIsNew(false);
    },
    [entries]
  );

  const startNew = useCallback(() => {
    setSelectedIndex(null);
    setIsNew(true);
    setEditSchedule({
      id: "",
      cron: "0 * * * *",
      timezone: "UTC",
      trigger: { workflow: "" },
    });
  }, []);

  const handleSave = useCallback(async () => {
    if (!editSchedule?.id) return;
    setSaving(true);
    try {
      const clean = structuredClone(editSchedule);
      if (!clean.timezone) delete clean.timezone;
      if (clean.services && !Object.values(clean.services).some(Boolean))
        delete clean.services;
      if (clean.lock && !clean.lock.enabled) delete clean.lock;
      if (clean.trigger) {
        if (!clean.trigger.input || Object.keys(clean.trigger.input).length === 0)
          delete clean.trigger.input;
      }

      const filePath = isNew
        ? `schedules/${clean.id}.json`
        : entries[selectedIndex!].filePath;
      await api.writeFile(filePath, clean);
      showToast({ type: "success", message: `Schedule "${clean.id}" saved` });
      setIsNew(false);
      await loadFiles();
      await reload();
    } catch (err) {
      showToast({ type: "error", message: `Failed to save: ${err}` });
    } finally {
      setSaving(false);
    }
  }, [editSchedule, isNew, selectedIndex, entries, loadFiles, reload]);

  const handleDelete = useCallback(async () => {
    if (selectedIndex === null) return;
    const entry = entries[selectedIndex];
    if (!confirm(`Delete schedule "${entry.schedule.id}"?`)) return;
    try {
      await api.deleteFile(entry.filePath);
      showToast({ type: "success", message: `Schedule deleted` });
      setSelectedIndex(null);
      setEditSchedule(null);
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
    (patch: Partial<ScheduleConfig>) => {
      if (editSchedule) setEditSchedule({ ...editSchedule, ...patch });
    },
    [editSchedule]
  );

  const updateTrigger = useCallback(
    (patch: Partial<NonNullable<ScheduleConfig["trigger"]>>) => {
      if (!editSchedule) return;
      setEditSchedule({
        ...editSchedule,
        trigger: {
          ...editSchedule.trigger,
          workflow: editSchedule.trigger?.workflow ?? "",
          ...patch,
        },
      });
    },
    [editSchedule]
  );

  if (loading) {
    return <div className="p-6 text-sm text-gray-400">Loading schedules...</div>;
  }

  return (
    <div className="flex-1 flex flex-col min-h-0">
      <ViewHeader title="Schedules" subtitle="Cron-based scheduled workflow execution" />
      <div className="flex-1 flex min-h-0">
      {/* Schedule list */}
      <div className="w-80 border-r border-gray-200 overflow-y-auto">
        <div className="px-4 py-3 border-b border-gray-200 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-gray-800">
            Schedules ({entries.length})
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
              onClick={() => selectSchedule(index)}
              className={`w-full text-left px-4 py-2.5 hover:bg-gray-50 ${
                selectedIndex === index && !isNew ? "bg-blue-50" : ""
              }`}
            >
              <div className="text-sm font-medium text-gray-800 truncate">
                {entry.schedule.id}
              </div>
              <div className="text-xs text-gray-400 flex items-center gap-1">
                <Clock size={10} />
                {describeCron(entry.schedule.cron)}
              </div>
            </button>
          ))}
          {entries.length === 0 && (
            <div className="p-4 text-sm text-gray-400">No schedules configured.</div>
          )}
        </div>
      </div>

      {/* Schedule editor */}
      <div className="flex-1 overflow-y-auto p-6">
        {editSchedule ? (
          <div className="max-w-2xl space-y-5">
            <div className="flex items-center justify-between">
              <h3 className="text-lg font-semibold text-gray-900">
                {isNew ? "New Schedule" : editSchedule.id}
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
                  disabled={saving || !editSchedule.id}
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
                value={editSchedule.id}
                onChange={(e) => update({ id: e.target.value })}
                className="input-field font-mono"
                placeholder="e.g. cleanup-tokens"
              />
            </Field>

            {/* Cron + Preview */}
            <Field label="Cron Expression">
              <input
                type="text"
                value={editSchedule.cron}
                onChange={(e) => update({ cron: e.target.value })}
                className="input-field font-mono"
                placeholder="0 */6 * * *"
              />
              <div className="mt-1 text-xs text-gray-500">
                {describeCron(editSchedule.cron)}
              </div>
            </Field>

            {/* Timezone */}
            <Field label="Timezone">
              <select
                value={editSchedule.timezone ?? "UTC"}
                onChange={(e) => update({ timezone: e.target.value })}
                className="input-field"
              >
                {COMMON_TIMEZONES.map((tz) => (
                  <option key={tz} value={tz}>{tz}</option>
                ))}
              </select>
            </Field>

            {/* Lock */}
            <div className="border-t border-gray-200 pt-4">
              <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-3">
                Distributed Lock
              </h4>
              <div className="flex items-center gap-3 mb-3">
                <label className="flex items-center gap-2 text-sm text-gray-700 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={editSchedule.lock?.enabled ?? false}
                    onChange={(e) =>
                      update({
                        lock: { ...editSchedule.lock, enabled: e.target.checked },
                      })
                    }
                    className="rounded"
                  />
                  Enabled
                </label>
              </div>
              {editSchedule.lock?.enabled && (
                <div className="grid grid-cols-2 gap-3">
                  <Field label="Lock Service">
                    <select
                      value={editSchedule.services?.lock ?? ""}
                      onChange={(e) =>
                        update({
                          services: { ...editSchedule.services, lock: e.target.value },
                        })
                      }
                      className="input-field"
                    >
                      <option value="">Select service...</option>
                      {serviceNames.map((n) => (
                        <option key={n} value={n}>{n}</option>
                      ))}
                    </select>
                  </Field>
                  <Field label="TTL">
                    <input
                      type="text"
                      value={editSchedule.lock?.ttl ?? ""}
                      onChange={(e) =>
                        update({
                          lock: { ...editSchedule.lock, ttl: e.target.value || undefined },
                        })
                      }
                      className="input-field"
                      placeholder="e.g. 300s"
                    />
                  </Field>
                </div>
              )}
            </div>

            {/* Trigger */}
            <div className="border-t border-gray-200 pt-4">
              <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-3">
                Trigger
              </h4>
              <Field label="Workflow">
                <div className="flex items-center gap-2">
                  <select
                    value={editSchedule.trigger?.workflow ?? ""}
                    onChange={(e) => updateTrigger({ workflow: e.target.value })}
                    className="input-field flex-1"
                  >
                    <option value="">Select workflow...</option>
                    {(files?.workflows ?? []).map((wf) => {
                      const name = wf.replace(/^workflows\//, "").replace(/\.json$/, "");
                      return (
                        <option key={wf} value={name}>{name}</option>
                      );
                    })}
                  </select>
                  {editSchedule.trigger?.workflow && (
                    <button
                      onClick={() => goToWorkflow(editSchedule.trigger!.workflow)}
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
                  entries={editSchedule.trigger?.input ?? {}}
                  onChange={(input) =>
                    updateTrigger({
                      input: Object.keys(input).length > 0 ? input : undefined,
                    })
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
                {JSON.stringify(editSchedule, null, 2)}
              </pre>
            </div>
          </div>
        ) : (
          <div className="text-sm text-gray-400">
            Select a schedule to edit or click "New" to create one.
          </div>
        )}
      </div>
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
              placeholder="value"
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
