import { useEffect, useState } from "react";
import { Circle, Play } from "lucide-react";
import * as api from "@/api/client";
import { useEditorStore } from "@/stores/editor";

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

export function TestsView() {
  const files = useEditorStore((s) => s.files);
  const [suites, setSuites] = useState<{ path: string; suite: TestSuite }[]>([]);
  const [selectedSuite, setSelectedSuite] = useState<{ path: string; suite: TestSuite } | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!files?.tests) return;
    setLoading(true);
    Promise.all(
      files.tests.map(async (path) => {
        const data = await api.readFile(path);
        return { path, suite: data as TestSuite };
      })
    )
      .then(setSuites)
      .finally(() => setLoading(false));
  }, [files?.tests]);

  if (loading) {
    return <div className="p-6 text-sm text-gray-400">Loading tests...</div>;
  }

  return (
    <div className="flex-1 flex min-h-0">
      {/* Suite list */}
      <div className="w-72 border-r border-gray-200 overflow-y-auto">
        <div className="px-4 py-3 border-b border-gray-200">
          <h2 className="text-sm font-semibold text-gray-800">Test Suites ({suites.length})</h2>
        </div>
        <div className="divide-y divide-gray-100">
          {suites.map(({ path, suite }) => (
            <button
              key={path}
              onClick={() => setSelectedSuite({ path, suite })}
              className={`w-full text-left px-4 py-2.5 hover:bg-gray-50 ${
                selectedSuite?.path === path ? "bg-blue-50" : ""
              }`}
            >
              <div className="text-sm font-medium text-gray-800">{suite.id}</div>
              <div className="text-xs text-gray-400">
                {suite.workflow} · {suite.tests.length} test{suite.tests.length !== 1 ? "s" : ""}
              </div>
            </button>
          ))}
          {suites.length === 0 && (
            <div className="p-4 text-sm text-gray-400">No test suites found.</div>
          )}
        </div>
      </div>

      {/* Test cases */}
      <div className="flex-1 overflow-y-auto p-6">
        {selectedSuite ? (
          <SuiteDetail suite={selectedSuite.suite} />
        ) : (
          <div className="text-sm text-gray-400">Select a test suite to view cases.</div>
        )}
      </div>
    </div>
  );
}

function SuiteDetail({ suite }: { suite: TestSuite }) {
  return (
    <div className="max-w-2xl space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-lg font-semibold text-gray-900">{suite.id}</h3>
          <p className="text-sm text-gray-500">Workflow: {suite.workflow}</p>
        </div>
        <button
          className="flex items-center gap-1.5 px-3 py-1.5 bg-blue-600 text-white text-sm rounded hover:bg-blue-700 disabled:opacity-50"
          title="Test execution requires noda test CLI (not yet available via API)"
          disabled
        >
          <Play size={14} />
          Run All
        </button>
      </div>

      <div className="border border-gray-200 rounded-lg divide-y divide-gray-100">
        {suite.tests.map((tc, i) => (
          <div key={i} className="px-4 py-3">
            <div className="flex items-center gap-2">
              <Circle size={14} className="text-gray-300 shrink-0" />
              <span className="text-sm font-medium text-gray-800">{tc.name}</span>
            </div>

            <div className="mt-2 ml-6 space-y-2">
              {tc.input && (
                <div>
                  <label className="text-xs text-gray-400">Input</label>
                  <pre className="mt-0.5 p-2 bg-gray-50 rounded text-xs text-gray-600 overflow-x-auto">
                    {JSON.stringify(tc.input, null, 2)}
                  </pre>
                </div>
              )}
              {tc.mocks && Object.keys(tc.mocks).length > 0 && (
                <div>
                  <label className="text-xs text-gray-400">Mocks</label>
                  <pre className="mt-0.5 p-2 bg-gray-50 rounded text-xs text-gray-600 overflow-x-auto">
                    {JSON.stringify(tc.mocks, null, 2)}
                  </pre>
                </div>
              )}
              {tc.expect && (
                <div>
                  <label className="text-xs text-gray-400">Expected</label>
                  <pre className="mt-0.5 p-2 bg-gray-50 rounded text-xs text-gray-600 overflow-x-auto">
                    {JSON.stringify(tc.expect, null, 2)}
                  </pre>
                </div>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
