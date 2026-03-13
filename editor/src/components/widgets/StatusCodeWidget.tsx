import { useState, useCallback } from "react";
import type { WidgetProps } from "@rjsf/utils";
import Editor from "@monaco-editor/react";

const STATUS_CODES = [
  { value: "200", label: "200 — OK" },
  { value: "201", label: "201 — Created" },
  { value: "204", label: "204 — No Content" },
  { value: "301", label: "301 — Moved Permanently" },
  { value: "302", label: "302 — Found" },
  { value: "304", label: "304 — Not Modified" },
  { value: "400", label: "400 — Bad Request" },
  { value: "401", label: "401 — Unauthorized" },
  { value: "403", label: "403 — Forbidden" },
  { value: "404", label: "404 — Not Found" },
  { value: "409", label: "409 — Conflict" },
  { value: "422", label: "422 — Unprocessable Entity" },
  { value: "429", label: "429 — Too Many Requests" },
  { value: "500", label: "500 — Internal Server Error" },
  { value: "502", label: "502 — Bad Gateway" },
  { value: "503", label: "503 — Service Unavailable" },
];

export function StatusCodeWidget(props: WidgetProps) {
  const { value, onChange, label, required, readonly } = props;
  const strValue = value != null ? String(value) : "";
  const [exprMode, setExprMode] = useState(
    strValue.includes("{{")
  );

  const handleSelectChange = useCallback(
    (e: React.ChangeEvent<HTMLSelectElement>) => {
      onChange(e.target.value);
    },
    [onChange]
  );

  return (
    <div className="mb-2">
      <div className="flex items-center justify-between mb-1">
        <label className="text-sm font-medium text-gray-700">
          {label}
          {required && <span className="text-red-500 ml-0.5">*</span>}
        </label>
        <button
          type="button"
          onClick={() => setExprMode(!exprMode)}
          className="text-xs text-blue-500 hover:text-blue-700"
        >
          {exprMode ? "Select" : "Expression"}
        </button>
      </div>
      {exprMode ? (
        <div className="border border-gray-300 rounded overflow-hidden">
          <Editor
            height="60px"
            language="plaintext"
            value={strValue}
            onChange={(v) => onChange(v ?? "")}
            options={{
              minimap: { enabled: false },
              lineNumbers: "off",
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
              readOnly: readonly,
            }}
          />
        </div>
      ) : (
        <select
          value={strValue}
          onChange={handleSelectChange}
          disabled={readonly}
          className="w-full px-3 py-1.5 text-sm border border-gray-300 rounded bg-white focus:outline-none focus:ring-2 focus:ring-blue-400 focus:border-transparent"
        >
          <option value="">Select status code...</option>
          {STATUS_CODES.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
      )}
    </div>
  );
}
