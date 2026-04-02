import { describe, it, expect } from "vitest";
import { deriveSchemaFromConfig } from "./configDerivedSchema";

describe("deriveSchemaFromConfig", () => {
  describe("transform.set", () => {
    it("derives object schema with matching keys from fields config", () => {
      const config = { fields: { name: "Alice", age: 30 } };
      const result = deriveSchemaFromConfig("transform.set", config);
      expect(result).toEqual({
        type: "object",
        properties: {
          name: {},
          age: {},
        },
      });
    });

    it("derives object schema with no properties when fields is empty", () => {
      const config = { fields: {} };
      const result = deriveSchemaFromConfig("transform.set", config);
      expect(result).toEqual({ type: "object", properties: {} });
    });

    it("returns null when fields is missing", () => {
      const result = deriveSchemaFromConfig("transform.set", {});
      expect(result).toBeNull();
    });

    it("returns null when fields is not an object", () => {
      const result = deriveSchemaFromConfig("transform.set", {
        fields: "invalid",
      });
      expect(result).toBeNull();
    });
  });

  describe("db.create", () => {
    it("derives object schema with matching keys from values config", () => {
      const config = { values: { id: 1, email: "test@example.com" } };
      const result = deriveSchemaFromConfig("db.create", config);
      expect(result).toEqual({
        type: "object",
        properties: {
          id: {},
          email: {},
        },
      });
    });

    it("returns null when values is missing", () => {
      const result = deriveSchemaFromConfig("db.create", {});
      expect(result).toBeNull();
    });
  });

  describe("db.update", () => {
    it("derives object schema with matching keys from values config", () => {
      const config = { values: { status: "active", updatedAt: "2026-01-01" } };
      const result = deriveSchemaFromConfig("db.update", config);
      expect(result).toEqual({
        type: "object",
        properties: {
          status: {},
          updatedAt: {},
        },
      });
    });

    it("returns null when values is missing", () => {
      const result = deriveSchemaFromConfig("db.update", {});
      expect(result).toBeNull();
    });
  });

  describe("unknown node types", () => {
    it("returns null for an unknown node type", () => {
      const result = deriveSchemaFromConfig("unknown.node", { fields: {} });
      expect(result).toBeNull();
    });

    it("returns null for control.if which has no deriver", () => {
      const result = deriveSchemaFromConfig("control.if", {
        condition: "true",
      });
      expect(result).toBeNull();
    });

    it("returns null for an empty node type string", () => {
      const result = deriveSchemaFromConfig("", {});
      expect(result).toBeNull();
    });
  });
});
