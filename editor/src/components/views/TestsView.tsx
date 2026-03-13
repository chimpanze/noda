import { useEffect, useState, useCallback } from "react";
import {
  Circle,
  Plus,
  Trash2,
  ChevronDown,
  ChevronRight,
  CheckCircle,
  XCircle,
  Loader2,
  Play,
} from "lucide-react";
import { ViewHeader } from "@/components/layout/ViewHeader";
import Editor from "@monaco-editor/react";
import * as api from "@/api/client";
import type { TestRunResult } from "@/api/client";
import { useEditorStore } from "@/stores/editor";
import { showToast } from "@/utils/toast";

interface TestSuite {
  id: string;
  workflow: string;
  tests: TestCase[];
}

interface TestCase {
  name: string;
  input?: Record<string, unknown>;
  mocks?: Record<string, unknown>;
  expect?: {
    status?: string;
    output?: Record<string, unknown>;
    error_node?: string;
  };
}

type TestStatus = "idle" | "running" | "passed" | "failed";

export function TestsView() {
  const files = useEditorStore((s) => s.files);
  const loadFiles = useEditorStore((s) => s.loadFiles);
  const [suites, setSuites] = useState<{ path: string; suite: TestSuite }[]>(
    [],
  );
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [editSuite, setEditSuite] = useState<TestSuite | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [isNew, setIsNew] = useState(false);

  // Test runner state
  const [testResults, setTestResults] = useState<Map<string, TestRunResult>>(
    new Map(),
  );
  const [testStatuses, setTestStatuses] = useState<Map<string, TestStatus>>(
    new Map(),
  );
  const [runningAll, setRunningAll] = useState(false);

  const reload = useCallback(async () => {
    if (!files?.tests) return;
    setLoading(true);
    try {
      const results = await Promise.all(
        files.tests.map(async (path) => {
          const data = await api.readFile(path);
          return { path, suite: data as TestSuite };
        }),
      );
      setSuites(results);
    } finally {
      setLoading(false);
    }
  }, [files?.tests]);

  useEffect(() => {
    reload();
  }, [reload]);

  const selectSuite = useCallback(
    (path: string) => {
      const entry = suites.find((s) => s.path === path);
      if (entry) {
        setSelectedPath(path);
        setEditSuite(structuredClone(entry.suite));
        setIsNew(false);
        setTestResults(new Map());
        setTestStatuses(new Map());
      }
    },
    [suites],
  );

  const startNew = useCallback(() => {
    setSelectedPath(null);
    setIsNew(true);
    setEditSuite({
      id: "",
      workflow: "",
      tests: [{ name: "test case 1" }],
    });
    setTestResults(new Map());
    setTestStatuses(new Map());
  }, []);

  const handleSave = useCallback(async () => {
    if (!editSuite?.id) return;
    setSaving(true);
    try {
      const filePath = isNew ? `tests/${editSuite.id}.json` : selectedPath!;
      await api.writeFile(filePath, editSuite);
      showToast({
        type: "success",
        message: `Test suite "${editSuite.id}" saved`,
      });
      setIsNew(false);
      await loadFiles();
      await reload();
      setSelectedPath(filePath);
    } catch (err) {
      showToast({ type: "error", message: `Failed to save: ${err}` });
    } finally {
      setSaving(false);
    }
  }, [editSuite, isNew, selectedPath, loadFiles, reload]);

  const handleDelete = useCallback(async () => {
    if (!selectedPath) return;
    if (!confirm("Delete this test suite?")) return;
    try {
      await api.deleteFile(selectedPath);
      showToast({ type: "success", message: "Test suite deleted" });
      setSelectedPath(null);
      setEditSuite(null);
      await loadFiles();
      await reload();
    } catch (err) {
      showToast({ type: "error", message: `Failed to delete: ${err}` });
    }
  }, [selectedPath, loadFiles, reload]);

  const handleRunAll = useCallback(async () => {
    if (!selectedPath || isNew) return;
    setRunningAll(true);

    // Mark all as running
    const statuses = new Map<string, TestStatus>();
    for (const tc of editSuite?.tests ?? []) {
      statuses.set(tc.name, "running");
    }
    setTestStatuses(new Map(statuses));
    setTestResults(new Map());

    try {
      const results = await api.runTests(selectedPath);
      const newResults = new Map<string, TestRunResult>();
      const newStatuses = new Map<string, TestStatus>();
      for (const r of results) {
        newResults.set(r.case_name, r);
        newStatuses.set(r.case_name, r.passed ? "passed" : "failed");
      }
      setTestResults(newResults);
      setTestStatuses(newStatuses);

      const passed = results.filter((r) => r.passed).length;
      const failed = results.length - passed;
      if (failed === 0) {
        showToast({ type: "success", message: `All ${passed} tests passed` });
      } else {
        showToast({
          type: "error",
          message: `${failed} failed, ${passed} passed`,
        });
      }
    } catch (err) {
      showToast({ type: "error", message: `Test run failed: ${err}` });
      // Reset statuses
      setTestStatuses(new Map());
    } finally {
      setRunningAll(false);
    }
  }, [selectedPath, isNew, editSuite?.tests]);

  const handleRunSingle = useCallback(
    async (testName: string) => {
      if (!selectedPath || isNew) return;

      setTestStatuses((prev) => new Map(prev).set(testName, "running"));

      try {
        const results = await api.runTests(selectedPath);
        const match = results.find((r) => r.case_name === testName);
        if (match) {
          setTestResults((prev) => new Map(prev).set(testName, match));
          setTestStatuses((prev) =>
            new Map(prev).set(testName, match.passed ? "passed" : "failed"),
          );
        }
      } catch (err) {
        setTestStatuses((prev) => new Map(prev).set(testName, "idle"));
        showToast({ type: "error", message: `Test run failed: ${err}` });
      }
    },
    [selectedPath, isNew],
  );

  const updateSuite = useCallback(
    (patch: Partial<TestSuite>) => {
      if (editSuite) setEditSuite({ ...editSuite, ...patch });
    },
    [editSuite],
  );

  const updateTest = useCallback(
    (index: number, patch: Partial<TestCase>) => {
      if (!editSuite) return;
      const tests = editSuite.tests.map((t, i) =>
        i === index ? { ...t, ...patch } : t,
      );
      setEditSuite({ ...editSuite, tests });
    },
    [editSuite],
  );

  const addTest = useCallback(() => {
    if (!editSuite) return;
    setEditSuite({
      ...editSuite,
      tests: [
        ...editSuite.tests,
        { name: `test case ${editSuite.tests.length + 1}` },
      ],
    });
  }, [editSuite]);

  const removeTest = useCallback(
    (index: number) => {
      if (!editSuite) return;
      setEditSuite({
        ...editSuite,
        tests: editSuite.tests.filter((_, i) => i !== index),
      });
    },
    [editSuite],
  );

  if (loading) {
    return <div className="p-6 text-sm text-gray-400">Loading tests...</div>;
  }

  return (
    <div className="flex-1 flex flex-col min-h-0">
      <ViewHeader
        title="Tests"
        subtitle="Workflow test suites and execution results"
      />
      <div className="flex-1 flex min-h-0">
        {/* Suite list */}
        <div className="w-72 border-r border-gray-200 overflow-y-auto">
          <div className="px-4 py-3 border-b border-gray-200 flex items-center justify-between">
            <h2 className="text-sm font-semibold text-gray-800">
              Test Suites ({suites.length})
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
            {suites.map(({ path, suite }) => (
              <button
                key={path}
                onClick={() => selectSuite(path)}
                className={`w-full text-left px-4 py-2.5 hover:bg-gray-50 ${
                  selectedPath === path && !isNew ? "bg-blue-50" : ""
                }`}
              >
                <div className="text-sm font-medium text-gray-800">
                  {suite.id}
                </div>
                <div className="text-xs text-gray-400">
                  {suite.workflow} &middot; {suite.tests.length} test
                  {suite.tests.length !== 1 ? "s" : ""}
                </div>
              </button>
            ))}
            {suites.length === 0 && (
              <div className="p-4 text-sm text-gray-400">
                No test suites found.
              </div>
            )}
          </div>
        </div>

        {/* Suite editor */}
        <div className="flex-1 overflow-y-auto p-6">
          {editSuite ? (
            <div className="max-w-3xl space-y-5">
              <div className="flex items-center justify-between">
                <h3 className="text-lg font-semibold text-gray-900">
                  {isNew ? "New Test Suite" : editSuite.id}
                </h3>
                <div className="flex items-center gap-2">
                  {!isNew && (
                    <button
                      onClick={handleRunAll}
                      disabled={runningAll}
                      className="px-3 py-1.5 text-sm text-green-700 border border-green-300 rounded hover:bg-green-50 disabled:opacity-50 flex items-center gap-1"
                    >
                      {runningAll ? (
                        <Loader2 size={14} className="animate-spin" />
                      ) : (
                        <Play size={14} />
                      )}
                      Run All
                    </button>
                  )}
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
                    disabled={saving || !editSuite.id || !editSuite.workflow}
                    className="px-4 py-1.5 text-sm text-white bg-blue-500 rounded hover:bg-blue-600 disabled:opacity-50"
                  >
                    {saving ? "Saving..." : "Save"}
                  </button>
                </div>
              </div>

              {/* Suite metadata */}
              <div className="grid grid-cols-2 gap-3">
                <Field label="Suite ID">
                  <input
                    type="text"
                    value={editSuite.id}
                    onChange={(e) => updateSuite({ id: e.target.value })}
                    className="input-field font-mono"
                    placeholder="e.g. test-create-task"
                  />
                </Field>
                <Field label="Workflow">
                  <select
                    value={editSuite.workflow}
                    onChange={(e) => updateSuite({ workflow: e.target.value })}
                    className="input-field"
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
                </Field>
              </div>

              {/* Test cases */}
              <div className="border-t border-gray-200 pt-4">
                <div className="flex items-center justify-between mb-3">
                  <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider">
                    Test Cases ({editSuite.tests.length})
                  </h4>
                  <button
                    onClick={addTest}
                    className="flex items-center gap-1 text-xs text-blue-500 hover:text-blue-700"
                  >
                    <Plus size={12} />
                    Add Test
                  </button>
                </div>

                <div className="space-y-3">
                  {editSuite.tests.map((tc, i) => (
                    <TestCaseEditor
                      key={i}
                      testCase={tc}
                      index={i}
                      onChange={(patch) => updateTest(i, patch)}
                      onDelete={() => removeTest(i)}
                      onRun={() => handleRunSingle(tc.name)}
                      status={testStatuses.get(tc.name) ?? "idle"}
                      result={testResults.get(tc.name)}
                      canRun={!isNew && !!selectedPath}
                    />
                  ))}
                </div>
              </div>
            </div>
          ) : (
            <div className="text-sm text-gray-400">
              Select a test suite to edit or click "New" to create one.
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function StatusIcon({ status }: { status: TestStatus }) {
  switch (status) {
    case "running":
      return (
        <Loader2 size={14} className="text-blue-500 animate-spin shrink-0" />
      );
    case "passed":
      return <CheckCircle size={14} className="text-green-500 shrink-0" />;
    case "failed":
      return <XCircle size={14} className="text-red-500 shrink-0" />;
    default:
      return <Circle size={14} className="text-gray-300 shrink-0" />;
  }
}

function TestCaseEditor({
  testCase,
  index,
  onChange,
  onDelete,
  onRun,
  status,
  result,
  canRun,
}: {
  testCase: TestCase;
  index: number;
  onChange: (patch: Partial<TestCase>) => void;
  onDelete: () => void;
  onRun: () => void;
  status: TestStatus;
  result?: TestRunResult;
  canRun: boolean;
}) {
  const [expanded, setExpanded] = useState(true);
  const [showResult, setShowResult] = useState(false);

  // Auto-expand results when they arrive
  useEffect(() => {
    if (!result || result.passed) return;
    const timer = setTimeout(() => setShowResult(true), 0);
    return () => clearTimeout(timer);
  }, [result]);

  return (
    <div className="border border-gray-200 rounded-lg overflow-hidden">
      <div className="flex items-center gap-2 px-3 py-2 bg-gray-50">
        <button
          onClick={() => setExpanded(!expanded)}
          className="text-gray-400"
        >
          {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        </button>
        <StatusIcon status={status} />
        <input
          type="text"
          value={testCase.name}
          onChange={(e) => onChange({ name: e.target.value })}
          className="flex-1 text-sm font-medium text-gray-800 bg-transparent border-none focus:outline-none"
          placeholder="Test case name"
        />
        {result?.duration && (
          <span className="text-[10px] text-gray-400 font-mono">
            {result.duration}
          </span>
        )}
        {canRun && (
          <button
            onClick={onRun}
            disabled={status === "running"}
            className="text-green-500 hover:text-green-700 disabled:opacity-50"
            title="Run this test"
          >
            <Play size={12} />
          </button>
        )}
        <span className="text-xs text-gray-400">#{index + 1}</span>
        <button
          onClick={onDelete}
          className="text-red-400 hover:text-red-600"
          title="Remove test case"
        >
          <Trash2 size={12} />
        </button>
      </div>

      {/* Test result panel */}
      {result && (
        <div
          className={`px-3 py-2 text-xs border-t ${
            result.passed
              ? "bg-green-50 border-green-200 text-green-800"
              : "bg-red-50 border-red-200 text-red-800"
          }`}
        >
          <div className="flex items-center justify-between">
            <span className="font-medium">
              {result.passed ? "PASSED" : "FAILED"}
              {result.duration && ` (${result.duration})`}
            </span>
            {!result.passed && (
              <button
                onClick={() => setShowResult(!showResult)}
                className="text-red-600 hover:text-red-800"
              >
                {showResult ? "Hide Details" : "Show Details"}
              </button>
            )}
          </div>
          {result.error && (
            <div className="mt-1 font-mono text-[11px] whitespace-pre-wrap">
              {result.error}
            </div>
          )}
          {showResult && !result.passed && (
            <div className="mt-2 space-y-2">
              <div>
                <span className="font-medium">Expected:</span>
                <pre className="mt-0.5 p-1.5 bg-white/60 rounded font-mono text-[10px] whitespace-pre-wrap">
                  {JSON.stringify(result.expected, null, 2)}
                </pre>
              </div>
              <div>
                <span className="font-medium">Actual:</span>
                <pre className="mt-0.5 p-1.5 bg-white/60 rounded font-mono text-[10px] whitespace-pre-wrap">
                  {JSON.stringify(result.actual, null, 2)}
                </pre>
              </div>
            </div>
          )}
        </div>
      )}

      {expanded && (
        <div className="p-3 space-y-3">
          <JsonField
            label="Input"
            value={testCase.input}
            onChange={(input) => onChange({ input })}
          />
          <JsonField
            label="Mocks"
            value={testCase.mocks}
            onChange={(mocks) => onChange({ mocks })}
          />
          <JsonField
            label="Expected"
            value={testCase.expect}
            onChange={(expect) =>
              onChange({ expect: expect as TestCase["expect"] })
            }
          />
        </div>
      )}
    </div>
  );
}

function JsonField({
  label,
  value,
  onChange,
}: {
  label: string;
  value: Record<string, unknown> | undefined;
  onChange: (value: Record<string, unknown> | undefined) => void;
}) {
  const [text, setText] = useState(value ? JSON.stringify(value, null, 2) : "");
  const [error, setError] = useState<string | null>(null);

  const handleChange = useCallback(
    (v: string | undefined) => {
      const raw = v ?? "";
      setText(raw);
      if (!raw.trim()) {
        setError(null);
        onChange(undefined);
        return;
      }
      try {
        const parsed = JSON.parse(raw);
        setError(null);
        onChange(parsed);
      } catch (e) {
        setError((e as Error).message);
      }
    },
    [onChange],
  );

  return (
    <div>
      <div className="flex items-center justify-between mb-1">
        <label className="text-xs font-medium text-gray-400 uppercase">
          {label}
        </label>
        {error && <span className="text-xs text-red-500">Invalid JSON</span>}
      </div>
      <div className="border border-gray-200 rounded overflow-hidden">
        <Editor
          height="80px"
          language="json"
          value={text}
          onChange={handleChange}
          options={{
            minimap: { enabled: false },
            fontSize: 12,
            scrollBeyondLastLine: false,
            wordWrap: "on",
            lineNumbers: "off",
            folding: false,
            renderLineHighlight: "none",
            overviewRulerLanes: 0,
            overviewRulerBorder: false,
            scrollbar: { vertical: "hidden" },
          }}
        />
      </div>
    </div>
  );
}

function Field({
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
