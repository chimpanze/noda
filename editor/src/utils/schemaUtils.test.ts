import { describe, it, expect } from "vitest";
import { groupByFolder } from "./schemaUtils";
import type { SchemaInfo } from "@/types";

function schema(path: string): SchemaInfo {
  return { path, name: path.split("/").pop()! } as SchemaInfo;
}

describe("groupByFolder", () => {
  it("groups schemas by folder", () => {
    const schemas = [
      schema("schemas/users/create.json"),
      schema("schemas/users/update.json"),
      schema("schemas/tasks/create.json"),
    ];
    const groups = groupByFolder(schemas);
    expect(groups.size).toBe(2);
    expect(groups.get("users")).toHaveLength(2);
    expect(groups.get("tasks")).toHaveLength(1);
  });

  it("puts root schemas in empty-string folder", () => {
    const schemas = [schema("schemas/root.json")];
    const groups = groupByFolder(schemas);
    expect(groups.get("")).toHaveLength(1);
  });

  it("handles empty array", () => {
    const groups = groupByFolder([]);
    expect(groups.size).toBe(0);
  });

  it("handles nested folders", () => {
    const schemas = [schema("schemas/api/v2/user.json")];
    const groups = groupByFolder(schemas);
    expect(groups.has("api/v2")).toBe(true);
  });
});
