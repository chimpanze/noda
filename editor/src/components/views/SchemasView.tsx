import { useEffect, useState } from "react";
import Editor from "@monaco-editor/react";
import * as api from "@/api/client";
import type { SchemaInfo } from "@/types";

export function SchemasView() {
  const [schemas, setSchemas] = useState<SchemaInfo[]>([]);
  const [selected, setSelected] = useState<SchemaInfo | null>(null);
  const [editorValue, setEditorValue] = useState("");
  const [parseError, setParseError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api.listSchemas().then((s) => {
      setSchemas(s);
      setLoading(false);
    });
  }, []);

  const selectSchema = (schema: SchemaInfo) => {
    setSelected(schema);
    setEditorValue(JSON.stringify(schema.schema, null, 2));
    setParseError(null);
  };

  const onEditorChange = (value: string | undefined) => {
    const v = value ?? "";
    setEditorValue(v);
    try {
      JSON.parse(v);
      setParseError(null);
    } catch (e) {
      setParseError((e as Error).message);
    }
  };

  if (loading) {
    return <div className="p-6 text-sm text-gray-400">Loading schemas...</div>;
  }

  return (
    <div className="flex-1 flex min-h-0">
      {/* Schema list */}
      <div className="w-64 border-r border-gray-200 overflow-y-auto">
        <div className="px-4 py-3 border-b border-gray-200">
          <h2 className="text-sm font-semibold text-gray-800">Schemas ({schemas.length})</h2>
        </div>
        <div className="divide-y divide-gray-100">
          {schemas.map((schema) => (
            <button
              key={schema.path}
              onClick={() => selectSchema(schema)}
              className={`w-full text-left px-4 py-2.5 hover:bg-gray-50 ${
                selected?.path === schema.path ? "bg-blue-50" : ""
              }`}
            >
              <div className="text-sm font-medium text-gray-800 truncate">
                {schema.path.replace(/^schemas\//, "").replace(/\.json$/, "")}
              </div>
              <div className="text-xs text-gray-400 truncate">{schema.path}</div>
            </button>
          ))}
          {schemas.length === 0 && (
            <div className="p-4 text-sm text-gray-400">No schemas defined.</div>
          )}
        </div>
      </div>

      {/* Schema editor */}
      <div className="flex-1 flex flex-col min-h-0">
        {selected ? (
          <>
            <div className="px-4 py-2 border-b border-gray-200 flex items-center justify-between bg-gray-50 shrink-0">
              <div>
                <span className="text-sm font-medium text-gray-800">{selected.path}</span>
              </div>
              {parseError ? (
                <span className="text-xs text-red-500">Invalid JSON: {parseError}</span>
              ) : (
                <span className="text-xs text-green-600">Valid JSON Schema</span>
              )}
            </div>
            <div className="flex-1 min-h-0">
              <Editor
                height="100%"
                language="json"
                value={editorValue}
                onChange={onEditorChange}
                options={{
                  minimap: { enabled: false },
                  fontSize: 13,
                  scrollBeyondLastLine: false,
                  wordWrap: "on",
                  formatOnPaste: true,
                }}
              />
            </div>
          </>
        ) : (
          <div className="flex-1 flex items-center justify-center text-sm text-gray-400">
            Select a schema to edit.
          </div>
        )}
      </div>
    </div>
  );
}
