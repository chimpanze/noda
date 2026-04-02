import type { Execution, TraceEvent } from "../types";

export const CORE_NODE_TYPES = new Set([
  "transform.set", "transform.map", "transform.filter", "transform.reduce",
  "transform.flatten", "transform.pick", "transform.omit", "transform.merge",
  "control.if", "control.switch", "control.loop",
  "workflow.run", "workflow.output",
  "response.json", "response.redirect", "response.error",
  "util.log", "util.uuid", "util.delay", "util.timestamp",
  "event.emit", "upload.handle", "ws.send", "sse.send",
  "wasm.send", "wasm.query",
]);

export interface GeneratedTestCase {
  name: string;
  input: Record<string, unknown>;
  auth?: { user_id: string; roles: string[]; claims?: Record<string, unknown> };
  mocks: Record<string, { output: unknown; output_name: string }>;
  expect: { status: string; output?: Record<string, unknown> };
}

export function generateTestCase(
  execution: Execution,
  coreNodeTypes: Set<string>,
): GeneratedTestCase {
  // Extract input and auth from workflow:started event
  const startEvent = execution.events.find((e) => e.type === "workflow:started");
  const startData = (startEvent?.data ?? {}) as Record<string, unknown>;
  const input = (startData.input as Record<string, unknown>) ?? {};
  const auth = startData.auth as GeneratedTestCase["auth"] | undefined;

  // Build mocks for non-core nodes
  const mocks: GeneratedTestCase["mocks"] = {};
  const completedNodes = execution.events.filter(
    (e): e is TraceEvent & { node_id: string; node_type: string } =>
      e.type === "node:completed" && !!e.node_id && !!e.node_type,
  );

  for (const event of completedNodes) {
    if (coreNodeTypes.has(event.node_type)) continue;
    const nodeData = execution.nodeData.get(event.node_id);
    mocks[event.node_id] = {
      output: nodeData?.data ?? null,
      output_name: event.output ?? "success",
    };
  }

  return {
    name: `${execution.workflowId} - ${execution.traceId.slice(0, 8)}`,
    input,
    auth,
    mocks,
    expect: {
      status: execution.status === "completed" ? "success" : "error",
    },
  };
}

export function testCaseToJSON(
  testCase: GeneratedTestCase,
  workflowId: string,
  existingSuite?: { id: string; tests: unknown[] },
): string {
  const suite = existingSuite
    ? { ...existingSuite }
    : { id: `test-${workflowId}`, workflow: workflowId, tests: [] as unknown[] };

  const testEntry: Record<string, unknown> = {
    name: testCase.name,
    input: testCase.input,
  };
  if (testCase.auth) testEntry.auth = testCase.auth;
  if (Object.keys(testCase.mocks).length > 0) testEntry.mocks = testCase.mocks;
  testEntry.expect = testCase.expect;

  if (existingSuite) {
    suite.tests = [...suite.tests, testEntry];
  } else {
    suite.tests = [testEntry];
  }

  return JSON.stringify(suite, null, 2);
}
