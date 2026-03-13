import { useState, useCallback, useEffect } from "react";
import type { FieldProps } from "@rjsf/utils";
import Editor from "@monaco-editor/react";
import * as api from "@/api/client";
import { SchemaSelect } from "@/components/widgets/SchemaSelect";
import type { SchemaInfo } from "@/types";

type Mode = "ref" | "inline";

export function SchemaRefField(props: FieldProps) {
  const { formData, onChange, schema, name, fieldPathId } = props;
  const title = schema.title ?? name;
  const path = fieldPathId.path;

  const [schemas, setSchemas] = useState<SchemaInfo[]>([]);
  const [mode, setMode] = useState<Mode>(() => detectMode(formData));
  const [inlineText, setInlineText] = useState(() =>
    formData && !formData["$ref"] ? JSON.stringify(formData, null, 2) : ""
  );
  const [jsonError, setJsonError] = useState<string | null>(null);

  useEffect(() => {
    api.listSchemas().then(setSchemas).catch(() => {});
  }, []);

  // Sync mode when formData changes externally
  useEffect(() => {
    setMode(detectMode(formData));
  }, [formData]);

  const currentRef =
    formData && typeof formData === "object" && formData["$ref"]
      ? String(formData["$ref"])
      : "";

  const handleRefChange = useCallback(
    (ref: string) => {
      if (ref) {
        onChange({ $ref: ref }, path);
      } else {
        onChange(undefined, path);
      }
    },
    [onChange, path]
  );

  const handleInlineChange = useCallback(
    (value: string | undefined) => {
      const raw = value ?? "";
      setInlineText(raw);
      if (!raw.trim()) {
        setJsonError(null);
        onChange(undefined, path);
        return;
      }
      try {
        const parsed = JSON.parse(raw);
        setJsonError(null);
        onChange(parsed, path);
      } catch (e) {
        setJsonError((e as Error).message);
      }
    },
    [onChange, path]
  );

  const switchMode = useCallback(
    (newMode: Mode) => {
      setMode(newMode);
      if (newMode === "ref") {
        // Clear inline, keep any existing ref or reset
        setInlineText("");
        setJsonError(null);
        if (!currentRef) {
          onChange(undefined, path);
        }
      } else {
        // Switch to inline — serialize current formData if it's not a ref
        if (formData && !formData["$ref"]) {
          setInlineText(JSON.stringify(formData, null, 2));
        } else {
          setInlineText("");
          onChange(undefined, path);
        }
      }
    },
    [currentRef, formData, onChange, path]
  );

  return (
    <div className="mb-2">
      <div className="flex items-center justify-between mb-1">
        <label className="text-sm font-medium text-gray-700">{title}</label>
        <div className="flex items-center gap-1">
          <button
            type="button"
            onClick={() => switchMode("ref")}
            className={`px-1.5 py-0.5 text-[10px] rounded ${
              mode === "ref"
                ? "bg-blue-100 text-blue-700 font-medium"
                : "text-gray-400 hover:text-gray-600"
            }`}
          >
            Schema ref
          </button>
          <button
            type="button"
            onClick={() => switchMode("inline")}
            className={`px-1.5 py-0.5 text-[10px] rounded ${
              mode === "inline"
                ? "bg-blue-100 text-blue-700 font-medium"
                : "text-gray-400 hover:text-gray-600"
            }`}
          >
            Inline JSON
          </button>
        </div>
      </div>
      {schema.description && (
        <p className="text-xs text-gray-400 mb-1.5">{schema.description}</p>
      )}

      {mode === "ref" ? (
        <SchemaSelect
          schemas={schemas}
          value={currentRef}
          onChange={handleRefChange}
          className="w-full px-3 py-1.5 text-sm font-mono border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-400 focus:border-transparent"
          placeholder="Select schema..."
        />
      ) : (
        <div>
          {jsonError && (
            <p className="text-xs text-red-500 mb-1">Invalid JSON</p>
          )}
          <div className="border border-gray-200 rounded overflow-hidden">
            <Editor
              height="120px"
              language="json"
              value={inlineText}
              onChange={handleInlineChange}
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
                scrollbar: { vertical: "auto" },
              }}
            />
          </div>
        </div>
      )}
    </div>
  );
}

function detectMode(formData: unknown): Mode {
  if (
    formData &&
    typeof formData === "object" &&
    !Array.isArray(formData) &&
    "$ref" in (formData as Record<string, unknown>)
  ) {
    return "ref";
  }
  // Default to ref mode for empty/new fields
  if (!formData || Object.keys(formData as Record<string, unknown>).length === 0) {
    return "ref";
  }
  return "inline";
}
