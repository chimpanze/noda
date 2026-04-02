import { describe, it, expect } from "vitest";
import { generateTestCase, testCaseToJSON, CORE_NODE_TYPES } from "./testGenerator";
import type { Execution } from "../types";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeExecution(overrides: Partial<Execution> = {}): Execution {
  return {
    traceId: "abcdef1234567890",
    workflowId: "test-workflow",
    status: "completed",
    startedAt: "2026-04-02T10:00:00Z",
    events: [],
    nodeStates: new Map(),
    nodeData: new Map(),
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// generateTestCase
// ---------------------------------------------------------------------------

describe("generateTestCase", () => {
  it("extracts input and auth, mocks plugin nodes, skips core nodes", () => {
    const execution = makeExecution({
      traceId: "trace-abc-xyz-123",
      workflowId: "create-user",
      status: "completed",
      events: [
        {
          type: "workflow:started",
          timestamp: "2026-04-02T10:00:00Z",
          trace_id: "trace-abc-xyz-123",
          workflow_id: "create-user",
          data: {
            input: { name: "Alice", email: "alice@example.com" },
            auth: { user_id: "user-1", roles: ["admin"], claims: { org: "acme" } },
          },
        },
        {
          type: "node:completed",
          timestamp: "2026-04-02T10:00:01Z",
          trace_id: "trace-abc-xyz-123",
          workflow_id: "create-user",
          node_id: "set-1",
          node_type: "transform.set",
          output: "success",
        },
        {
          type: "node:completed",
          timestamp: "2026-04-02T10:00:02Z",
          trace_id: "trace-abc-xyz-123",
          workflow_id: "create-user",
          node_id: "db-create-1",
          node_type: "db.create",
          output: "success",
        },
        {
          type: "node:completed",
          timestamp: "2026-04-02T10:00:03Z",
          trace_id: "trace-abc-xyz-123",
          workflow_id: "create-user",
          node_id: "resp-1",
          node_type: "response.json",
          output: "success",
        },
      ],
      nodeData: new Map([
        ["set-1", { output: "success", data: { name: "Alice" } }],
        ["db-create-1", { output: "success", data: { id: 42, name: "Alice" } }],
        ["resp-1", { output: "success", data: null }],
      ]),
    });

    const result = generateTestCase(execution, CORE_NODE_TYPES);

    // Input and auth extracted correctly
    expect(result.input).toEqual({ name: "Alice", email: "alice@example.com" });
    expect(result.auth).toEqual({
      user_id: "user-1",
      roles: ["admin"],
      claims: { org: "acme" },
    });

    // Plugin node (db.create) is mocked
    expect(result.mocks).toHaveProperty("db-create-1");
    expect(result.mocks["db-create-1"]).toEqual({
      output: { id: 42, name: "Alice" },
      output_name: "success",
    });

    // Core nodes (transform.set, response.json) are NOT mocked
    expect(result.mocks).not.toHaveProperty("set-1");
    expect(result.mocks).not.toHaveProperty("resp-1");

    // Status derived from execution.status
    expect(result.expect.status).toBe("success");

    // Name is workflowId + first 8 chars of traceId
    expect(result.name).toBe("create-user - trace-ab");
  });

  it("returns empty input and undefined auth when workflow:started has no data", () => {
    const execution = makeExecution({
      traceId: "aaaabbbbccccdddd",
      workflowId: "no-input-workflow",
      status: "failed",
      events: [
        {
          type: "workflow:started",
          timestamp: "2026-04-02T10:00:00Z",
          trace_id: "aaaabbbbccccdddd",
          workflow_id: "no-input-workflow",
          // data is intentionally absent
        },
      ],
    });

    const result = generateTestCase(execution, CORE_NODE_TYPES);

    expect(result.input).toEqual({});
    expect(result.auth).toBeUndefined();
    expect(result.mocks).toEqual({});
    expect(result.expect.status).toBe("error");
  });

  it("sets expect.status to error when execution failed", () => {
    const execution = makeExecution({ status: "failed" });
    const result = generateTestCase(execution, CORE_NODE_TYPES);
    expect(result.expect.status).toBe("error");
  });

  it("uses null for mock output when nodeData has no entry", () => {
    const execution = makeExecution({
      events: [
        {
          type: "workflow:started",
          timestamp: "2026-04-02T10:00:00Z",
          trace_id: "abcdef1234567890",
          workflow_id: "test-workflow",
        },
        {
          type: "node:completed",
          timestamp: "2026-04-02T10:00:01Z",
          trace_id: "abcdef1234567890",
          workflow_id: "test-workflow",
          node_id: "http-1",
          node_type: "http.get",
          output: "ok",
        },
      ],
      nodeData: new Map(), // no entry for http-1
    });

    const result = generateTestCase(execution, CORE_NODE_TYPES);
    expect(result.mocks["http-1"]).toEqual({ output: null, output_name: "ok" });
  });
});

// ---------------------------------------------------------------------------
// testCaseToJSON
// ---------------------------------------------------------------------------

describe("testCaseToJSON", () => {
  it("produces valid JSON with correct test suite structure", () => {
    const testCase = {
      name: "my-workflow - abcdef12",
      input: { userId: "u1" },
      auth: { user_id: "u1", roles: ["viewer"] },
      mocks: {
        "db-1": { output: { rows: [] }, output_name: "success" },
      },
      expect: { status: "success" as const },
    };

    const json = testCaseToJSON(testCase, "my-workflow");
    const parsed = JSON.parse(json);

    expect(parsed.id).toBe("test-my-workflow");
    expect(parsed.workflow).toBe("my-workflow");
    expect(Array.isArray(parsed.tests)).toBe(true);
    expect(parsed.tests).toHaveLength(1);

    const entry = parsed.tests[0];
    expect(entry.name).toBe("my-workflow - abcdef12");
    expect(entry.input).toEqual({ userId: "u1" });
    expect(entry.auth).toEqual({ user_id: "u1", roles: ["viewer"] });
    expect(entry.mocks).toEqual({
      "db-1": { output: { rows: [] }, output_name: "success" },
    });
    expect(entry.expect).toEqual({ status: "success" });
  });

  it("omits auth field when undefined", () => {
    const testCase = {
      name: "wf - 00000000",
      input: {},
      mocks: {},
      expect: { status: "success" as const },
    };

    const json = testCaseToJSON(testCase, "wf");
    const parsed = JSON.parse(json);
    expect(parsed.tests[0]).not.toHaveProperty("auth");
  });

  it("omits mocks field when empty", () => {
    const testCase = {
      name: "wf - 00000000",
      input: {},
      mocks: {},
      expect: { status: "success" as const },
    };

    const json = testCaseToJSON(testCase, "wf");
    const parsed = JSON.parse(json);
    expect(parsed.tests[0]).not.toHaveProperty("mocks");
  });

  it("appends to an existing suite", () => {
    const existingSuite = {
      id: "test-my-workflow",
      workflow: "my-workflow",
      tests: [{ name: "existing test", input: {}, expect: { status: "success" } }],
    };

    const testCase = {
      name: "new test",
      input: { x: 1 },
      mocks: {},
      expect: { status: "error" as const },
    };

    const json = testCaseToJSON(testCase, "my-workflow", existingSuite);
    const parsed = JSON.parse(json);

    expect(parsed.tests).toHaveLength(2);
    expect(parsed.tests[0].name).toBe("existing test");
    expect(parsed.tests[1].name).toBe("new test");
  });
});
