import { useState, useCallback, useRef, useEffect } from "react";
import type { WidgetProps } from "@rjsf/utils";
import Editor, { type Monaco } from "@monaco-editor/react";
import type { editor as MonacoEditor } from "monaco-editor";
import { registerExpressionLanguage } from "@/utils/expressionLanguage";
import * as api from "@/api/client";
import { useEditorStore } from "@/stores/editor";
import { ExpressionAutocomplete } from "@/components/widgets/ExpressionAutocomplete";

// RJSF custom widget for expression fields (string fields that may contain {{ }})
export function ExpressionWidget(props: WidgetProps) {
  const { value, onChange, label, required, readonly } = props;
  const [useMonaco, setUseMonaco] = useState(
    typeof value === "string" && value.includes("{{")
  );
  const [validationError, setValidationError] = useState<string | null>(null);
  const validateTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const activeWorkflowPath = useEditorStore((s) => s.activeWorkflowPath);
  const selectedNodeId = useEditorStore((s) => s.selectedNodeId);
  const workflowName = activeWorkflowPath
    ?.replace(/^workflows\//, "")
    .replace(/\.json$/, "");

  const handleMonacoMount = useCallback((_editor: MonacoEditor.IStandaloneCodeEditor, monaco: Monaco) => {
    registerExpressionLanguage(monaco);
  }, []);

  // Debounced expression validation
  const validateExpression = useCallback((val: string) => {
    if (validateTimer.current) clearTimeout(validateTimer.current);
    if (!val.includes("{{")) {
      setValidationError(null);
      return;
    }
    validateTimer.current = setTimeout(async () => {
      try {
        const result = await api.validateExpression(val);
        setValidationError(result.valid ? null : (result.error ?? "Invalid expression"));
      } catch {
        // API not available — skip validation
        setValidationError(null);
      }
    }, 500);
  }, []);

  // Cleanup timer
  useEffect(() => {
    return () => {
      if (validateTimer.current) clearTimeout(validateTimer.current);
    };
  }, []);

  const handleMonacoChange = useCallback(
    (val: string | undefined) => {
      const v = val ?? "";
      onChange(v);
      validateExpression(v);
    },
    [onChange, validateExpression]
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
        <div className={`border rounded overflow-hidden ${validationError ? "border-red-400" : "border-gray-300"}`}>
          <Editor
            height="60px"
            language="plaintext"
            value={typeof value === "string" ? value : ""}
            onChange={handleMonacoChange}
            onMount={handleMonacoMount}
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
              quickSuggestions: true,
              suggestOnTriggerCharacters: true,
            }}
          />
        </div>
      ) : (
        <ExpressionAutocomplete
          value={typeof value === "string" ? value : ""}
          onChange={(v) => {
            onChange(v);
            validateExpression(v);
            if (v.includes("{{") && !useMonaco) {
              setUseMonaco(true);
            }
          }}
          workflow={workflowName}
          node={selectedNodeId ?? undefined}
          className={`w-full px-3 py-1.5 text-sm border rounded focus:outline-none focus:ring-2 focus:ring-blue-400 focus:border-transparent ${
            validationError ? "border-red-400" : "border-gray-300"
          }`}
          placeholder={`Enter ${label?.toLowerCase() ?? "value"}...`}
        />
      )}
      {validationError && (
        <p className="mt-0.5 text-xs text-red-500 truncate" title={validationError}>
          {validationError}
        </p>
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
  const handleMount = useCallback((_editor: MonacoEditor.IStandaloneCodeEditor, monaco: Monaco) => {
    registerExpressionLanguage(monaco);
  }, []);

  return (
    <div className="border border-gray-300 rounded overflow-hidden">
      <Editor
        height={height}
        language="plaintext"
        value={value}
        onChange={(v) => onChange(v ?? "")}
        onMount={handleMount}
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
          quickSuggestions: true,
          suggestOnTriggerCharacters: true,
        }}
      />
    </div>
  );
}
