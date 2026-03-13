import { useState, useCallback, useRef, useEffect } from "react";
import * as api from "@/api/client";
import { useEditorStore } from "@/stores/editor";

interface ExpressionAutocompleteProps {
  value: string;
  onChange: (value: string) => void;
  workflow?: string;
  node?: string;
  className?: string;
  placeholder?: string;
}

interface Suggestion {
  label: string;
  type: string;
  description: string;
  insertText: string;
}

export function ExpressionAutocomplete({
  value,
  onChange,
  workflow,
  node,
  className = "input-field font-mono",
  placeholder,
}: ExpressionAutocompleteProps) {
  const vars = useEditorStore((s) => s.vars);
  const [showDropdown, setShowDropdown] = useState(false);
  const [suggestions, setSuggestions] = useState<Suggestion[]>([]);
  const [selectedIdx, setSelectedIdx] = useState(0);
  const [validationStatus, setValidationStatus] = useState<"valid" | "invalid" | "idle">("idle");
  const inputRef = useRef<HTMLInputElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(null);

  // Fetch suggestions from expression context
  const fetchSuggestions = useCallback(async () => {
    if (!workflow) return;
    try {
      const ctx = await api.getExpressionContext(workflow, node);
      const items: Suggestion[] = [];

      for (const v of ctx.variables) {
        items.push({
          label: v.name,
          type: v.type,
          description: v.description,
          insertText: v.name,
        });
      }
      for (const f of ctx.functions) {
        items.push({
          label: f.name,
          type: "function",
          description: f.description,
          insertText: f.name,
        });
      }
      for (const u of ctx.upstream) {
        items.push({
          label: u.ref,
          type: u.node_type,
          description: `Output from ${u.node_id}`,
          insertText: u.ref,
        });
      }

      // Add $var() suggestions from shared variables
      for (const v of vars) {
        items.push({
          label: `$var('${v.name}')`,
          type: "variable",
          description: v.value,
          insertText: `$var('${v.name}')`,
        });
      }

      setSuggestions(items);
    } catch {
      // Silently fail
    }
  }, [workflow, node]);

  // Validate expression on change (debounced)
  useEffect(() => {
    if (!value || !value.includes("{{")) {
      setValidationStatus("idle");
      return;
    }

    // Extract expression content
    const match = value.match(/\{\{(.+?)\}\}/);
    if (!match) {
      setValidationStatus("idle");
      return;
    }

    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(async () => {
      try {
        const result = await api.validateExpression(match[1].trim());
        setValidationStatus(result.valid ? "valid" : "invalid");
      } catch {
        setValidationStatus("idle");
      }
    }, 500);

    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, [value]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      if (e.key === " " && e.ctrlKey) {
        e.preventDefault();
        fetchSuggestions().then(() => setShowDropdown(true));
        return;
      }

      if (!showDropdown) return;

      if (e.key === "ArrowDown") {
        e.preventDefault();
        setSelectedIdx((prev) => Math.min(prev + 1, suggestions.length - 1));
      } else if (e.key === "ArrowUp") {
        e.preventDefault();
        setSelectedIdx((prev) => Math.max(prev - 1, 0));
      } else if (e.key === "Enter" && showDropdown && suggestions.length > 0) {
        e.preventDefault();
        insertSuggestion(suggestions[selectedIdx]);
      } else if (e.key === "Escape") {
        setShowDropdown(false);
      }
    },
    [showDropdown, suggestions, selectedIdx, fetchSuggestions]
  );

  const insertSuggestion = useCallback(
    (suggestion: Suggestion) => {
      const input = inputRef.current;
      if (!input) return;

      const cursorPos = input.selectionStart ?? value.length;
      const before = value.slice(0, cursorPos);
      const after = value.slice(cursorPos);

      // If inside {{ }}, replace from the last {{ to cursor
      const openIdx = before.lastIndexOf("{{");
      if (openIdx >= 0) {
        const prefix = value.slice(0, openIdx);
        const closeIdx = after.indexOf("}}");
        const suffix = closeIdx >= 0 ? after.slice(closeIdx + 2) : after;
        onChange(`${prefix}{{ ${suggestion.insertText} }}${suffix}`);
      } else {
        // Insert as a new expression
        onChange(`${before}{{ ${suggestion.insertText} }}${after}`);
      }

      setShowDropdown(false);
    },
    [value, onChange]
  );

  const handleInput = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const newValue = e.target.value;
      onChange(newValue);

      // Auto-show suggestions when {{ is typed
      const cursorPos = e.target.selectionStart ?? newValue.length;
      const before = newValue.slice(0, cursorPos);
      if (before.endsWith("{{") || before.endsWith("{{ ")) {
        fetchSuggestions().then(() => setShowDropdown(true));
      }
    },
    [onChange, fetchSuggestions]
  );

  // Filter suggestions based on current input
  const filteredSuggestions = (() => {
    if (!showDropdown) return [];
    const cursorPos = inputRef.current?.selectionStart ?? value.length;
    const before = value.slice(0, cursorPos);
    const openIdx = before.lastIndexOf("{{");
    if (openIdx < 0) return suggestions;

    const partial = before.slice(openIdx + 2).trim().toLowerCase();
    if (!partial) return suggestions;
    return suggestions.filter(
      (s) =>
        s.label.toLowerCase().includes(partial) ||
        s.description.toLowerCase().includes(partial)
    );
  })();

  return (
    <div className="relative">
      <div className="flex items-center gap-1.5">
        <input
          ref={inputRef}
          type="text"
          value={value}
          onChange={handleInput}
          onKeyDown={handleKeyDown}
          onBlur={() => {
            setTimeout(() => setShowDropdown(false), 150);
          }}
          className={className}
          placeholder={placeholder}
        />
        {validationStatus !== "idle" && (
          <span
            className={`w-2 h-2 rounded-full shrink-0 ${
              validationStatus === "valid" ? "bg-green-500" : "bg-red-500"
            }`}
            title={validationStatus === "valid" ? "Expression valid" : "Expression invalid"}
          />
        )}
      </div>

      {showDropdown && filteredSuggestions.length > 0 && (
        <div
          ref={dropdownRef}
          className="absolute z-20 mt-1 w-full max-h-48 overflow-y-auto bg-white border border-gray-200 rounded shadow-lg"
        >
          {filteredSuggestions.map((s, i) => (
            <button
              key={s.label}
              type="button"
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => insertSuggestion(s)}
              className={`w-full text-left px-3 py-1.5 text-sm flex items-center gap-2 ${
                i === selectedIdx ? "bg-blue-50 text-blue-800" : "hover:bg-gray-50"
              }`}
            >
              <span className="px-1 py-0.5 text-[10px] font-mono bg-gray-100 rounded text-gray-500 shrink-0">
                {s.type}
              </span>
              <span className="font-mono text-gray-800">{s.label}</span>
              <span className="text-xs text-gray-400 truncate ml-auto">{s.description}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
