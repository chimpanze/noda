import { describe, it, expect } from "vitest";
import { detectSchemaDiff, diffToLabel } from "./dataDiff";
import type { JSONSchema } from "../types";

describe("detectSchemaDiff", () => {
  it("returns hasChanges: false for identical schemas", () => {
    const schema: JSONSchema = {
      type: "object",
      properties: {
        name: { type: "string" },
        age: { type: "number" },
      },
    };
    const result = detectSchemaDiff(schema, schema);
    expect(result.hasChanges).toBe(false);
    expect(result.addedKeys).toEqual([]);
    expect(result.removedKeys).toEqual([]);
    expect(result.changedTypes).toEqual([]);
  });

  it("detects added keys", () => {
    const previous: JSONSchema = {
      type: "object",
      properties: {
        name: { type: "string" },
      },
    };
    const current: JSONSchema = {
      type: "object",
      properties: {
        name: { type: "string" },
        email: { type: "string" },
      },
    };
    const result = detectSchemaDiff(previous, current);
    expect(result.hasChanges).toBe(true);
    expect(result.addedKeys).toEqual(["email"]);
    expect(result.removedKeys).toEqual([]);
    expect(result.changedTypes).toEqual([]);
  });

  it("detects removed keys", () => {
    const previous: JSONSchema = {
      type: "object",
      properties: {
        name: { type: "string" },
        phone: { type: "string" },
      },
    };
    const current: JSONSchema = {
      type: "object",
      properties: {
        name: { type: "string" },
      },
    };
    const result = detectSchemaDiff(previous, current);
    expect(result.hasChanges).toBe(true);
    expect(result.addedKeys).toEqual([]);
    expect(result.removedKeys).toEqual(["phone"]);
    expect(result.changedTypes).toEqual([]);
  });

  it("detects changed property types", () => {
    const previous: JSONSchema = {
      type: "object",
      properties: {
        count: { type: "string" },
      },
    };
    const current: JSONSchema = {
      type: "object",
      properties: {
        count: { type: "number" },
      },
    };
    const result = detectSchemaDiff(previous, current);
    expect(result.hasChanges).toBe(true);
    expect(result.changedTypes).toEqual([{ key: "count", from: "string", to: "number" }]);
    expect(result.addedKeys).toEqual([]);
    expect(result.removedKeys).toEqual([]);
  });

  it("detects multiple changes at once", () => {
    const previous: JSONSchema = {
      type: "object",
      properties: {
        name: { type: "string" },
        age: { type: "string" },
        old: { type: "boolean" },
      },
    };
    const current: JSONSchema = {
      type: "object",
      properties: {
        name: { type: "string" },
        age: { type: "number" },
        newField: { type: "string" },
      },
    };
    const result = detectSchemaDiff(previous, current);
    expect(result.hasChanges).toBe(true);
    expect(result.addedKeys).toEqual(["newField"]);
    expect(result.removedKeys).toEqual(["old"]);
    expect(result.changedTypes).toEqual([{ key: "age", from: "string", to: "number" }]);
  });

  it("detects non-object root type changes", () => {
    const previous: JSONSchema = { type: "string" };
    const current: JSONSchema = { type: "object" };
    const result = detectSchemaDiff(previous, current);
    expect(result.hasChanges).toBe(true);
    expect(result.changedTypes).toEqual([{ key: "$root", from: "string", to: "object" }]);
  });

  it("returns no changes for identical non-object types", () => {
    const schema: JSONSchema = { type: "string" };
    const result = detectSchemaDiff(schema, schema);
    expect(result.hasChanges).toBe(false);
  });

  it("handles objects with no properties", () => {
    const previous: JSONSchema = { type: "object" };
    const current: JSONSchema = { type: "object", properties: { foo: { type: "string" } } };
    const result = detectSchemaDiff(previous, current);
    expect(result.hasChanges).toBe(true);
    expect(result.addedKeys).toEqual(["foo"]);
  });
});

describe("diffToLabel", () => {
  it("returns null when there are no changes", () => {
    expect(
      diffToLabel({ addedKeys: [], removedKeys: [], changedTypes: [], hasChanges: false }),
    ).toBeNull();
  });

  it("formats added keys with + prefix", () => {
    const label = diffToLabel({
      addedKeys: ["email", "phone"],
      removedKeys: [],
      changedTypes: [],
      hasChanges: true,
    });
    expect(label).toBe("+email, +phone");
  });

  it("formats removed keys with - prefix", () => {
    const label = diffToLabel({
      addedKeys: [],
      removedKeys: ["old", "deprecated"],
      changedTypes: [],
      hasChanges: true,
    });
    expect(label).toBe("-old, -deprecated");
  });

  it("formats changed types with arrow notation", () => {
    const label = diffToLabel({
      addedKeys: [],
      removedKeys: [],
      changedTypes: [{ key: "count", from: "string", to: "number" }],
      hasChanges: true,
    });
    expect(label).toBe("count: string→number");
  });

  it("combines all parts with semicolons", () => {
    const label = diffToLabel({
      addedKeys: ["newField"],
      removedKeys: ["oldField"],
      changedTypes: [{ key: "age", from: "string", to: "number" }],
      hasChanges: true,
    });
    expect(label).toBe("+newField; -oldField; age: string→number");
  });
});
