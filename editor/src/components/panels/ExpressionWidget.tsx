import { useState, useCallback } from "react";
import type { WidgetProps } from "@rjsf/utils";
import Editor from "@monaco-editor/react";

// RJSF custom widget for expression fields (string fields that may contain {{ }})
export function ExpressionWidget(props: WidgetProps) {
  const { value, onChange, label, required, readonly } = props;
  const [useMonaco, setUseMonaco] = useState(
    typeof value === "string" && value.includes("{{")
  );

  const handleMonacoChange = useCallback(
    (val: string | undefined) => {
      onChange(val ?? "");
    },
    [onChange]
  );

  const handleInputChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const v = e.target.value;
      onChange(v);
      // Auto-switch to Monaco if user types {{
      if (v.includes("{{") && !useMonaco) {
        setUseMonaco(true);
      }
    },
    [onChange, useMonaco]
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
          onClick={() => setUseMonaco(!useMonaco)}
          className="text-xs text-blue-500 hover:text-blue-700"
        >
          {useMonaco ? "Plain text" : "Expression"}
        </button>
      </div>
      {useMonaco ? (
        <div className="border border-gray-300 rounded overflow-hidden">
          <Editor
            height="60px"
            language="plaintext"
            value={typeof value === "string" ? value : ""}
            onChange={handleMonacoChange}
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
          type="text"
          value={typeof value === "string" ? value : ""}
          onChange={handleInputChange}
          readOnly={readonly}
          className="w-full px-3 py-1.5 text-sm border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-400 focus:border-transparent"
          placeholder={`Enter ${label?.toLowerCase() ?? "value"}...`}
        />
      )}
    </div>
  );
}

// Standalone expression editor (outside RJSF) for more complex use cases
export function ExpressionEditor({
  value,
  onChange,
  height = "80px",
  readOnly = false,
}: {
  value: string;
  onChange: (value: string) => void;
  height?: string;
  readOnly?: boolean;
}) {
  return (
    <div className="border border-gray-300 rounded overflow-hidden">
      <Editor
        height={height}
        language="plaintext"
        value={value}
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
          readOnly,
        }}
      />
    </div>
  );
}
