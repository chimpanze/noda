import { useState, useCallback } from "react";
import type { FieldProps } from "@rjsf/utils";
import Editor from "@monaco-editor/react";

type Mode = "expression" | "json";

function detectMode(value: unknown): Mode {
  if (typeof value === "string") return "expression";
  return "json";
}

export function FlexibleValueField(props: FieldProps) {
  const { formData, onChange, schema, name, fieldPathId } = props;
  const [mode, setMode] = useState<Mode>(detectMode(formData));
  const title = schema.title ?? name;
  const path = fieldPathId.path;

  const editorValue =
    mode === "json"
      ? typeof formData === "string"
        ? formData
        : JSON.stringify(formData ?? null, null, 2)
      : typeof formData === "string"
        ? formData
        : "";

  const handleChange = useCallback(
    (val: string | undefined) => {
      const v = val ?? "";
      if (mode === "json") {
        try {
          onChange(JSON.parse(v), path);
        } catch {
          // Keep raw string while user is typing invalid JSON
          onChange(v, path);
        }
      } else {
        onChange(v, path);
      }
    },
    [mode, onChange, path],
  );

  const switchMode = useCallback(
    (newMode: Mode) => {
      if (newMode === mode) return;
      if (newMode === "json" && typeof formData === "string" && formData) {
        try {
          onChange(JSON.parse(formData), path);
        } catch {
          // keep as-is
        }
      } else if (newMode === "expression" && typeof formData !== "string") {
        onChange(JSON.stringify(formData ?? null, null, 2), path);
      }
      setMode(newMode);
    },
    [mode, formData, onChange, path],
  );

  return (
    <div className="mb-2">
      <div className="flex items-center justify-between mb-1">
        <label className="text-sm font-medium text-gray-700">{title}</label>
        <div className="flex gap-1">
          <button
            type="button"
            onClick={() => switchMode("expression")}
            className={`text-xs px-1.5 py-0.5 rounded ${
              mode === "expression"
                ? "bg-blue-100 text-blue-700"
                : "text-gray-500 hover:text-gray-700"
            }`}
          >
            Expression
          </button>
          <button
            type="button"
            onClick={() => switchMode("json")}
            className={`text-xs px-1.5 py-0.5 rounded ${
              mode === "json"
                ? "bg-blue-100 text-blue-700"
                : "text-gray-500 hover:text-gray-700"
            }`}
          >
            JSON
          </button>
        </div>
      </div>
      <div className="border border-gray-300 rounded overflow-hidden">
        <Editor
          height={mode === "json" ? "120px" : "60px"}
          language={mode === "json" ? "json" : "plaintext"}
          value={editorValue}
          onChange={handleChange}
          options={{
            minimap: { enabled: false },
            lineNumbers: mode === "json" ? "on" : "off",
            glyphMargin: false,
            folding: false,
            scrollBeyondLastLine: false,
            renderLineHighlight: "none",
            overviewRulerLanes: 0,
            hideCursorInOverviewRuler: true,
            overviewRulerBorder: false,
            scrollbar: { vertical: "hidden", horizontal: "auto" },
            wordWrap: "on",
            fontSize: 13,
          }}
        />
      </div>
    </div>
  );
}
