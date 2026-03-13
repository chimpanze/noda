import { useState, useRef, useEffect } from "react";
import { Braces } from "lucide-react";
import { useEditorStore } from "@/stores/editor";

interface VarPickerButtonProps {
  onSelect: (varRef: string) => void;
  currentValue?: string;
}

const varRefPattern = /\{\{\s*\$var\(\s*'([^']+)'\s*\)\s*\}\}/;

export function VarPickerButton({ onSelect, currentValue }: VarPickerButtonProps) {
  const vars = useEditorStore((s) => s.vars);
  const [open, setOpen] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);

  // Close on outside click
  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [open]);

  if (vars.length === 0) return null;

  // Check if current value is a $var() reference and show resolved value
  const match = currentValue?.match(varRefPattern);
  const resolvedHint = match
    ? vars.find((v) => v.name === match[1])?.value
    : undefined;

  return (
    <div className="relative inline-block" ref={dropdownRef}>
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="p-1 text-gray-400 hover:text-blue-500 rounded hover:bg-blue-50"
        title="Insert variable reference"
      >
        <Braces size={14} />
      </button>

      {resolvedHint !== undefined && (
        <span className="text-[11px] text-gray-400 ml-1">= {resolvedHint}</span>
      )}

      {open && (
        <div className="absolute z-30 mt-1 right-0 w-64 max-h-48 overflow-y-auto bg-white border border-gray-200 rounded shadow-lg">
          {vars.map((v) => (
            <button
              key={v.name}
              type="button"
              onClick={() => {
                onSelect(`{{ $var('${v.name}') }}`);
                setOpen(false);
              }}
              className="w-full text-left px-3 py-1.5 text-sm hover:bg-blue-50 flex items-center justify-between"
            >
              <span className="font-mono text-gray-800 truncate">{v.name}</span>
              <span className="text-xs text-gray-400 truncate ml-2">{v.value}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
