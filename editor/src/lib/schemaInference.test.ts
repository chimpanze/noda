import { describe, it, expect } from "vitest";
import {
  inferSchema,
  mergeSchemas,
  schemaToCompactLabel,
  dataToCompactLabel,
} from "./schemaInference";

// ---------------------------------------------------------------------------
// inferSchema
// ---------------------------------------------------------------------------

describe("inferSchema", () => {
  it("handles null", () => {
    expect(inferSchema(null)).toEqual({ type: "null" });
  });

  it("handles undefined", () => {
    expect(inferSchema(undefined)).toEqual({ type: "null" });
  });

  it("handles string", () => {
    expect(inferSchema("hello")).toEqual({ type: "string" });
  });

  it("handles number", () => {
    expect(inferSchema(42)).toEqual({ type: "number" });
    expect(inferSchema(3.14)).toEqual({ type: "number" });
  });

  it("handles boolean", () => {
    expect(inferSchema(true)).toEqual({ type: "boolean" });
    expect(inferSchema(false)).toEqual({ type: "boolean" });
  });

  it("handles empty array", () => {
    expect(inferSchema([])).toEqual({ type: "array" });
  });

  it("handles non-empty array — uses first element for items", () => {
    expect(inferSchema([1, 2, 3])).toEqual({
      type: "array",
      items: { type: "number" },
    });
  });

  it("handles array of objects", () => {
    expect(inferSchema([{ id: 1, name: "Alice" }])).toEqual({
      type: "array",
      items: {
        type: "object",
        properties: { id: { type: "number" }, name: { type: "string" } },
        required: ["id", "name"],
      },
    });
  });

  it("handles flat object", () => {
    expect(inferSchema({ a: 1, b: "two" })).toEqual({
      type: "object",
      properties: {
        a: { type: "number" },
        b: { type: "string" },
      },
      required: ["a", "b"],
    });
  });

  it("handles nested object", () => {
    const result = inferSchema({ user: { name: "Alice", age: 30 } });
    expect(result).toEqual({
      type: "object",
      properties: {
        user: {
          type: "object",
          properties: {
            name: { type: "string" },
            age: { type: "number" },
          },
          required: ["name", "age"],
        },
      },
      required: ["user"],
    });
  });

  it("handles object with array value", () => {
    const result = inferSchema({ items: [1, 2] });
    expect(result).toEqual({
      type: "object",
      properties: {
        items: { type: "array", items: { type: "number" } },
      },
      required: ["items"],
    });
  });
});

// ---------------------------------------------------------------------------
// mergeSchemas
// ---------------------------------------------------------------------------

describe("mergeSchemas", () => {
  it("same primitive type returns incoming", () => {
    expect(mergeSchemas({ type: "string" }, { type: "string" })).toEqual({
      type: "string",
    });
    expect(mergeSchemas({ type: "number" }, { type: "number" })).toEqual({
      type: "number",
    });
  });

  it("different types produce anyOf", () => {
    const result = mergeSchemas({ type: "string" }, { type: "number" });
    expect(result).toEqual({
      anyOf: [{ type: "string" }, { type: "number" }],
    });
  });

  it("adding a new key makes it non-required", () => {
    const existing = {
      type: "object",
      properties: { a: { type: "string" } },
      required: ["a"],
    };
    const incoming = {
      type: "object",
      properties: { a: { type: "string" }, b: { type: "number" } },
      required: ["a", "b"],
    };
    const result = mergeSchemas(existing, incoming);
    // "b" wasn't in existing.required, so it's dropped from intersection
    expect(result.required).toEqual(["a"]);
    expect(result.properties?.b).toEqual({ type: "number" });
  });

  it("required = intersection of both required arrays", () => {
    const a = {
      type: "object",
      properties: {
        x: { type: "string" },
        y: { type: "number" },
        z: { type: "boolean" },
      },
      required: ["x", "y", "z"],
    };
    const b = {
      type: "object",
      properties: {
        x: { type: "string" },
        y: { type: "number" },
      },
      required: ["x", "y"],
    };
    const result = mergeSchemas(a, b);
    expect(result.required?.sort()).toEqual(["x", "y"]);
  });

  it("widening type of an existing property", () => {
    const a = {
      type: "object",
      properties: { id: { type: "number" } },
      required: ["id"],
    };
    const b = {
      type: "object",
      properties: { id: { type: "string" } },
      required: ["id"],
    };
    const result = mergeSchemas(a, b);
    expect(result.properties?.id).toEqual({
      anyOf: [{ type: "number" }, { type: "string" }],
    });
  });

  it("merges array items", () => {
    const a = { type: "array", items: { type: "number" } };
    const b = { type: "array", items: { type: "number" } };
    expect(mergeSchemas(a, b)).toEqual({ type: "array", items: { type: "number" } });
  });

  it("widens array items when types differ", () => {
    const a = { type: "array", items: { type: "number" } };
    const b = { type: "array", items: { type: "string" } };
    const result = mergeSchemas(a, b);
    expect(result.items).toEqual({
      anyOf: [{ type: "number" }, { type: "string" }],
    });
  });

  it("handles empty array merging with typed array", () => {
    const a = { type: "array" };
    const b = { type: "array", items: { type: "string" } };
    const result = mergeSchemas(a, b);
    expect(result.type).toBe("array");
  });
});

// ---------------------------------------------------------------------------
// schemaToCompactLabel
// ---------------------------------------------------------------------------

describe("schemaToCompactLabel", () => {
  it("returns type name for primitives", () => {
    expect(schemaToCompactLabel({ type: "string" })).toBe("string");
    expect(schemaToCompactLabel({ type: "number" })).toBe("number");
    expect(schemaToCompactLabel({ type: "boolean" })).toBe("boolean");
    expect(schemaToCompactLabel({ type: "null" })).toBe("null");
  });

  it("returns [] for array without items", () => {
    expect(schemaToCompactLabel({ type: "array" })).toBe("[]");
  });

  it("returns [innerLabel] for array with items", () => {
    expect(schemaToCompactLabel({ type: "array", items: { type: "string" } })).toBe("[string]");
  });

  it("returns nested array labels", () => {
    expect(
      schemaToCompactLabel({
        type: "array",
        items: { type: "array", items: { type: "number" } },
      }),
    ).toBe("[[number]]");
  });

  it("returns {} for empty object", () => {
    expect(schemaToCompactLabel({ type: "object", properties: {} })).toBe("{}");
    expect(schemaToCompactLabel({ type: "object" })).toBe("{}");
  });

  it("returns {key} for single-key object", () => {
    expect(
      schemaToCompactLabel({
        type: "object",
        properties: { name: { type: "string" } },
      }),
    ).toBe("{name}");
  });

  it("returns {key1, key2} for two-key object", () => {
    expect(
      schemaToCompactLabel({
        type: "object",
        properties: { a: { type: "string" }, b: { type: "number" } },
      }),
    ).toBe("{a, b}");
  });

  it("returns {key1, key2, +N} for object with more than 2 keys", () => {
    expect(
      schemaToCompactLabel({
        type: "object",
        properties: {
          a: { type: "string" },
          b: { type: "number" },
          c: { type: "boolean" },
          d: { type: "null" },
        },
      }),
    ).toBe("{a, b, +2}");
  });

  it("handles anyOf with pipe separator", () => {
    expect(
      schemaToCompactLabel({
        anyOf: [{ type: "string" }, { type: "number" }],
      }),
    ).toBe("string | number");
  });

  it("handles anyOf with three variants", () => {
    expect(
      schemaToCompactLabel({
        anyOf: [{ type: "string" }, { type: "number" }, { type: "null" }],
      }),
    ).toBe("string | number | null");
  });

  it("returns unknown when no type or anyOf", () => {
    expect(schemaToCompactLabel({})).toBe("unknown");
  });
});

// ---------------------------------------------------------------------------
// dataToCompactLabel
// ---------------------------------------------------------------------------

describe("dataToCompactLabel", () => {
  it("returns null for null and undefined", () => {
    expect(dataToCompactLabel(null)).toBe("null");
    expect(dataToCompactLabel(undefined)).toBe("null");
  });

  it("returns quoted string for short strings (≤20 chars)", () => {
    expect(dataToCompactLabel("hello")).toBe('"hello"');
    expect(dataToCompactLabel("12345678901234567890")).toBe('"12345678901234567890"');
  });

  it("returns truncated string for long strings (>20 chars)", () => {
    expect(dataToCompactLabel("123456789012345678901")).toBe('"12345678901234567..."');
  });

  it("returns string representation for numbers", () => {
    expect(dataToCompactLabel(42)).toBe("42");
    expect(dataToCompactLabel(3.14)).toBe("3.14");
    expect(dataToCompactLabel(0)).toBe("0");
  });

  it("returns string representation for booleans", () => {
    expect(dataToCompactLabel(true)).toBe("true");
    expect(dataToCompactLabel(false)).toBe("false");
  });

  it("returns [N items] for arrays", () => {
    expect(dataToCompactLabel([])).toBe("[0 items]");
    expect(dataToCompactLabel([1, 2, 3])).toBe("[3 items]");
    expect(dataToCompactLabel([1])).toBe("[1 items]");
  });

  it("returns {} for empty object", () => {
    expect(dataToCompactLabel({})).toBe("{}");
  });

  it("returns {key} for single-key object", () => {
    expect(dataToCompactLabel({ name: "Alice" })).toBe("{name}");
  });

  it("returns {key1, key2} for two-key object", () => {
    expect(dataToCompactLabel({ a: 1, b: 2 })).toBe("{a, b}");
  });

  it("returns {key1, key2, +N} for object with more than 2 keys", () => {
    expect(dataToCompactLabel({ a: 1, b: 2, c: 3, d: 4 })).toBe("{a, b, +2}");
  });
});
