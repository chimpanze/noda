import { useState, useCallback } from "react";
import type { WidgetProps } from "@rjsf/utils";
import Editor from "@monaco-editor/react";

export function NumberWidget(props: WidgetProps) {
  const { value, onChange, label, required, readonly, schema } = props;
  const [exprMode, setExprMode] = useState(
    typeof value === "string" && value.includes("{{")
  );

  const step = schema.type === "integer" ? "1" : "any";

  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const raw = e.target.value;
      if (raw === "") {
        onChange(undefined);
      } else {
        const num = schema.type === "integer" ? parseInt(raw, 10) : parseFloat(raw);
        if (!isNaN(num)) onChange(num);
      }
    },
    [onChange, schema.type]
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
          {exprMode ? "Number" : "Expression"}
        </button>
      </div>
      {exprMode ? (
        <div className="border border-gray-300 rounded overflow-hidden">
          <Editor
            height="60px"
            language="plaintext"
            value={typeof value === "string" ? value : String(value ?? "")}
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
        <input
          type="number"
          step={step}
          value={value ?? ""}
          onChange={handleChange}
          readOnly={readonly}
          className="w-full px-3 py-1.5 text-sm border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-400 focus:border-transparent"
          placeholder={`Enter ${label?.toLowerCase() ?? "number"}...`}
        />
      )}
    </div>
  );
}
