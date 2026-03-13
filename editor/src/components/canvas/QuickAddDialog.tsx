import { useState, useMemo, useEffect, useRef, useCallback } from "react";
import { Search } from "lucide-react";
import { useEditorStore } from "@/stores/editor";
import { getCategoryStyle } from "./nodeStyles";

interface QuickAddDialogProps {
  x: number;
  y: number;
  onAdd: (nodeType: string) => void;
  onClose: () => void;
}

export function QuickAddDialog({ x, y, onAdd, onClose }: QuickAddDialogProps) {
  const nodeTypes = useEditorStore((s) => s.nodeTypes);
  const [search, setSearch] = useState("");
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  const filtered = useMemo(() => {
    if (!search) return nodeTypes;
    const lower = search.toLowerCase();
    return nodeTypes.filter(
      (nt) =>
        nt.type.toLowerCase().includes(lower) ||
        nt.name.toLowerCase().includes(lower),
    );
  }, [nodeTypes, search]);

  const updateSearch = useCallback((v: string) => {
    setSearch(v);
    setSelectedIndex(0);
  }, []);

  // Focus input on mount
  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  // Close on outside click
  useEffect(() => {
    const handle = (e: MouseEvent) => {
      const el = document.getElementById("quick-add-dialog");
      if (el && !el.contains(e.target as Element)) onClose();
    };
    document.addEventListener("mousedown", handle);
    return () => document.removeEventListener("mousedown", handle);
  }, [onClose]);

  // Scroll selected item into view
  useEffect(() => {
    const el = listRef.current?.children[selectedIndex] as
      | HTMLElement
      | undefined;
    el?.scrollIntoView({ block: "nearest" });
  }, [selectedIndex]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "ArrowDown") {
        e.preventDefault();
        setSelectedIndex((i) => Math.min(i + 1, filtered.length - 1));
      } else if (e.key === "ArrowUp") {
        e.preventDefault();
        setSelectedIndex((i) => Math.max(i - 1, 0));
      } else if (e.key === "Enter") {
        e.preventDefault();
        if (filtered[selectedIndex]) {
          onAdd(filtered[selectedIndex].type);
        }
      } else if (e.key === "Escape") {
        onClose();
      }
    },
    [filtered, selectedIndex, onAdd, onClose],
  );

  // Position: ensure dialog stays within viewport
  const style = useMemo(() => {
    const width = 280;
    const maxHeight = 320;
    const left = Math.min(x, window.innerWidth - width - 16);
    const top = Math.min(y, window.innerHeight - maxHeight - 16);
    return { left: Math.max(8, left), top: Math.max(8, top), width };
  }, [x, y]);

  return (
    <div
      id="quick-add-dialog"
      className="fixed z-50 bg-white border border-gray-200 rounded-lg shadow-xl flex flex-col"
      style={{ ...style, maxHeight: 320 }}
    >
      <div className="p-2 border-b border-gray-100">
        <div className="relative">
          <Search
            size={14}
            className="absolute left-2 top-1/2 -translate-y-1/2 text-gray-400"
          />
          <input
            ref={inputRef}
            type="text"
            placeholder="Search node types..."
            value={search}
            onChange={(e) => updateSearch(e.target.value)}
            onKeyDown={handleKeyDown}
            className="w-full pl-7 pr-2 py-1.5 text-sm border border-gray-200 rounded focus:outline-none focus:ring-1 focus:ring-blue-400"
          />
        </div>
      </div>

      <div ref={listRef} className="flex-1 overflow-y-auto py-1">
        {filtered.map((nt, i) => {
          const prefix = nt.type.split(".")[0];
          const style = getCategoryStyle(prefix);
          return (
            <button
              key={nt.type}
              onClick={() => onAdd(nt.type)}
              onMouseEnter={() => setSelectedIndex(i)}
              className={`w-full text-left px-3 py-1.5 text-sm flex items-center gap-2 ${
                i === selectedIndex
                  ? "bg-blue-50 text-blue-700"
                  : "text-gray-700 hover:bg-gray-50"
              }`}
            >
              <span
                className={`w-2 h-2 rounded-full shrink-0 ${style.bg} ${style.border} border`}
              />
              <span className="font-mono text-xs">{nt.type}</span>
              {nt.name !== nt.type && (
                <span className="text-gray-400 text-xs truncate ml-auto">
                  {nt.name}
                </span>
              )}
            </button>
          );
        })}
        {filtered.length === 0 && (
          <div className="p-3 text-xs text-gray-400 text-center">
            No matching node types.
          </div>
        )}
      </div>
    </div>
  );
}
