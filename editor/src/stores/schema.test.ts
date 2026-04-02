import { beforeEach, describe, expect, it, vi } from "vitest";
import { useSchemaStore } from "./schema";

// Mock the API client — loadStaticSchemas is not under test here
vi.mock("../api/client", () => ({
  fetchOutputSchemas: vi.fn().mockResolvedValue({}),
}));

// Provide a minimal localStorage shim for the node test environment
const localStorageStore: Record<string, string> = {};
const localStorageMock = {
  getItem: (key: string) => localStorageStore[key] ?? null,
  setItem: (key: string, value: string) => {
    localStorageStore[key] = value;
  },
  removeItem: (key: string) => {
    delete localStorageStore[key];
  },
  get length() {
    return Object.keys(localStorageStore).length;
  },
  key: (index: number) => Object.keys(localStorageStore)[index] ?? null,
  clear: () => {
    for (const k of Object.keys(localStorageStore)) {
      delete localStorageStore[k];
    }
  },
};
vi.stubGlobal("localStorage", localStorageMock);

beforeEach(() => {
  // Reset store state
  useSchemaStore.setState({
    staticSchemas: {},
    _learnedSchemas: {},
    _staleKeys: new Set(),
    _previousSchemas: {},
  });
  // Reset localStorage
  localStorageMock.clear();
});

// ---------------------------------------------------------------------------
// Priority resolution
// ---------------------------------------------------------------------------

describe("getNodeOutputSchema — priority resolution", () => {
  it("returns static schema when available", () => {
    useSchemaStore.setState({
      staticSchemas: {
        "cache.set": { type: "object", properties: { ok: { type: "boolean" } } },
      },
    });
    const result = useSchemaStore
      .getState()
      .getNodeOutputSchema("node1", "cache.set");
    expect(result?.source).toBe("static");
    expect(result?.schema.type).toBe("object");
  });

  it("returns config-derived schema when no static schema", () => {
    const result = useSchemaStore
      .getState()
      .getNodeOutputSchema("node1", "transform.set", {
        fields: { name: "test", age: "123" },
      });
    expect(result?.source).toBe("config-derived");
    expect(result?.schema.properties).toHaveProperty("name");
    expect(result?.schema.properties).toHaveProperty("age");
  });

  it("returns runtime-learned schema when no static or config-derived", () => {
    useSchemaStore.setState({
      _learnedSchemas: {
        node1: { type: "object", properties: { id: { type: "number" } } },
      },
    });
    const result = useSchemaStore
      .getState()
      .getNodeOutputSchema("node1", "db.query");
    expect(result?.source).toBe("runtime-learned");
  });

  it("prefers static over runtime-learned", () => {
    useSchemaStore.setState({
      staticSchemas: { "cache.set": { type: "object" } },
      _learnedSchemas: { node1: { type: "string" } },
    });
    const result = useSchemaStore
      .getState()
      .getNodeOutputSchema("node1", "cache.set");
    expect(result?.source).toBe("static");
  });

  it("returns null when no schema is available", () => {
    const result = useSchemaStore
      .getState()
      .getNodeOutputSchema("node1", "unknown.type");
    expect(result).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// Learned schema updates
// ---------------------------------------------------------------------------

describe("updateLearnedSchema", () => {
  it("infers and stores schema from runtime data", () => {
    useSchemaStore
      .getState()
      .updateLearnedSchema("wf1", "node1", { id: 1, name: "test" });
    const result = useSchemaStore
      .getState()
      .getNodeOutputSchema("node1", "db.query");
    expect(result?.source).toBe("runtime-learned");
    expect(result?.schema.properties).toHaveProperty("id");
    expect(result?.schema.properties).toHaveProperty("name");
  });

  it("merges new data with an existing learned schema", () => {
    useSchemaStore.getState().updateLearnedSchema("wf1", "node1", { id: 1 });
    useSchemaStore
      .getState()
      .updateLearnedSchema("wf1", "node1", { id: 2, email: "test@example.com" });
    const schema = useSchemaStore.getState()._learnedSchemas["node1"];
    expect(schema.properties).toHaveProperty("id");
    expect(schema.properties).toHaveProperty("email");
  });

  it("persists the merged schema to localStorage", () => {
    useSchemaStore
      .getState()
      .updateLearnedSchema("wf1", "node1", { count: 42 });
    const raw = localStorage.getItem("noda:schema:wf1:node1");
    expect(raw).not.toBeNull();
    const parsed = JSON.parse(raw!);
    expect(parsed.properties).toHaveProperty("count");
  });
});

// ---------------------------------------------------------------------------
// Staleness
// ---------------------------------------------------------------------------

describe("staleness", () => {
  it("markStale marks a node as stale", () => {
    useSchemaStore.setState({
      _learnedSchemas: { node1: { type: "object" } },
    });
    useSchemaStore.getState().markStale("wf1", "node1");
    const result = useSchemaStore
      .getState()
      .getNodeOutputSchema("node1", "db.query");
    expect(result?.stale).toBe(true);
  });

  it("updateLearnedSchema clears the stale flag", () => {
    useSchemaStore.setState({
      _learnedSchemas: { node1: { type: "object" } },
    });
    useSchemaStore.getState().markStale("wf1", "node1");
    useSchemaStore
      .getState()
      .updateLearnedSchema("wf1", "node1", { id: 1 });
    const result = useSchemaStore
      .getState()
      .getNodeOutputSchema("node1", "db.query");
    expect(result?.stale).toBe(false);
  });

  it("static schema is never stale even if node is in stale set", () => {
    useSchemaStore.setState({
      staticSchemas: { "cache.set": { type: "object" } },
    });
    useSchemaStore.getState().markStale("wf1", "node1");
    const result = useSchemaStore
      .getState()
      .getNodeOutputSchema("node1", "cache.set");
    expect(result?.stale).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Previous schema tracking
// ---------------------------------------------------------------------------

describe("getPreviousSchema", () => {
  it("returns null initially when no updates have occurred", () => {
    expect(useSchemaStore.getState().getPreviousSchema("node1")).toBeNull();
  });

  it("returns null after the first update (no prior schema to save)", () => {
    useSchemaStore
      .getState()
      .updateLearnedSchema("wf1", "node1", { id: 1 });
    // The first call has no pre-existing schema, so _previousSchemas stays empty
    expect(useSchemaStore.getState().getPreviousSchema("node1")).toBeNull();
  });

  it("returns the schema from before the most recent update", () => {
    useSchemaStore
      .getState()
      .updateLearnedSchema("wf1", "node1", { id: 1 });
    useSchemaStore
      .getState()
      .updateLearnedSchema("wf1", "node1", { id: 2, name: "test" });
    const prev = useSchemaStore.getState().getPreviousSchema("node1");
    expect(prev).not.toBeNull();
    // Previous schema had only "id"
    expect(prev?.properties).toHaveProperty("id");
    expect(prev?.properties).not.toHaveProperty("name");
  });

  it("previous schema is preserved across further updates", () => {
    useSchemaStore
      .getState()
      .updateLearnedSchema("wf1", "node1", { id: 1 });
    useSchemaStore
      .getState()
      .updateLearnedSchema("wf1", "node1", { id: 2, name: "test" });
    useSchemaStore
      .getState()
      .updateLearnedSchema("wf1", "node1", { id: 3, name: "foo", extra: true });
    // Previous schema should now be the merged result of the first two updates
    const prev = useSchemaStore.getState().getPreviousSchema("node1");
    expect(prev).not.toBeNull();
    expect(prev?.properties).toHaveProperty("id");
    expect(prev?.properties).toHaveProperty("name");
  });
});
