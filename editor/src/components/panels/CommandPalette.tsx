import { useState, useMemo, useEffect, useRef, useCallback } from "react";
import {
  Search,
  FileText,
  Globe,
  Box,
  Database,
  TestTube,
  LayoutGrid,
  Zap,
} from "lucide-react";
import { useEditorStore } from "@/stores/editor";
import type { ViewType } from "@/types";

interface PaletteItem {
  id: string;
  label: string;
  detail?: string;
  icon: React.ReactNode;
  action: () => void;
}

interface CommandPaletteProps {
  onClose: () => void;
}

export function CommandPalette({ onClose }: CommandPaletteProps) {
  const [search, setSearch] = useState("");
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  const updateSearch = useCallback((v: string) => {
    setSearch(v);
    setSelectedIndex(0);
  }, []);

  const files = useEditorStore((s) => s.files);
  const nodeTypes = useEditorStore((s) => s.nodeTypes);
  const setActiveView = useEditorStore((s) => s.setActiveView);
  const setActiveWorkflow = useEditorStore((s) => s.setActiveWorkflow);

  // Build all palette items
  const allItems = useMemo((): PaletteItem[] => {
    const items: PaletteItem[] = [];

    // Navigation actions
    const views: { view: ViewType; label: string; icon: React.ReactNode }[] = [
      {
        view: "workflows",
        label: "Go to Workflows",
        icon: <LayoutGrid size={14} />,
      },
      { view: "routes", label: "Go to Routes", icon: <Globe size={14} /> },
      {
        view: "services",
        label: "Go to Services",
        icon: <Database size={14} />,
      },
      { view: "schemas", label: "Go to Schemas", icon: <FileText size={14} /> },
      { view: "tests", label: "Go to Tests", icon: <TestTube size={14} /> },
      { view: "workers", label: "Go to Workers", icon: <Box size={14} /> },
      { view: "schedules", label: "Go to Schedules", icon: <Zap size={14} /> },
      {
        view: "connections",
        label: "Go to Connections",
        icon: <Zap size={14} />,
      },
      { view: "wasm", label: "Go to Wasm Runtimes", icon: <Box size={14} /> },
    ];

    for (const { view, label, icon } of views) {
      items.push({
        id: `nav:${view}`,
        label,
        icon,
        action: () => {
          setActiveView(view);
          onClose();
        },
      });
    }

    // Workflows
    for (const wf of files?.workflows ?? []) {
      const name = wf.replace(/^workflows\//, "").replace(/\.json$/, "");
      items.push({
        id: `wf:${wf}`,
        label: name,
        detail: "Workflow",
        icon: <LayoutGrid size={14} className="text-blue-500" />,
        action: () => {
          setActiveView("workflows");
          setActiveWorkflow(wf);
          onClose();
        },
      });
    }

    // Routes
    for (const rt of files?.routes ?? []) {
      const name = rt.replace(/^routes\//, "").replace(/\.json$/, "");
      items.push({
        id: `rt:${rt}`,
        label: name,
        detail: "Route",
        icon: <Globe size={14} className="text-green-500" />,
        action: () => {
          setActiveView("routes");
          onClose();
        },
      });
    }

    // Schemas
    for (const sc of files?.schemas ?? []) {
      const name = sc.replace(/^schemas\//, "").replace(/\.json$/, "");
      items.push({
        id: `sc:${sc}`,
        label: name,
        detail: "Schema",
        icon: <FileText size={14} className="text-purple-500" />,
        action: () => {
          setActiveView("schemas");
          onClose();
        },
      });
    }

    // Node types (for searching)
    for (const nt of nodeTypes) {
      items.push({
        id: `node:${nt.type}`,
        label: nt.type,
        detail: nt.name !== nt.type ? nt.name : "Node type",
        icon: <Box size={14} className="text-orange-500" />,
        action: () => {
          // Navigate to workflows — user can then drag from palette
          setActiveView("workflows");
          onClose();
        },
      });
    }

    return items;
  }, [files, nodeTypes, setActiveView, setActiveWorkflow, onClose]);

  // Filter by search
  const filtered = useMemo(() => {
    if (!search) return allItems.slice(0, 20); // Show top items when no search
    const lower = search.toLowerCase();
    return allItems
      .filter(
        (item) =>
          item.label.toLowerCase().includes(lower) ||
          (item.detail?.toLowerCase().includes(lower) ?? false),
      )
      .slice(0, 20);
  }, [allItems, search]);

  // Focus input on mount
  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  // Close on outside click or escape
  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    const handleClick = (e: MouseEvent) => {
      const el = document.getElementById("command-palette");
      if (el && !el.contains(e.target as Element)) onClose();
    };
    document.addEventListener("keydown", handleKey);
    document.addEventListener("mousedown", handleClick);
    return () => {
      document.removeEventListener("keydown", handleKey);
      document.removeEventListener("mousedown", handleClick);
    };
  }, [onClose]);

  // Scroll selected into view
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
        filtered[selectedIndex]?.action();
      }
    },
    [filtered, selectedIndex],
  );

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center pt-[15vh] bg-black/30">
      <div
        id="command-palette"
        className="bg-white rounded-xl shadow-2xl border border-gray-200 w-[520px] max-h-[400px] flex flex-col overflow-hidden"
      >
        <div className="p-3 border-b border-gray-100">
          <div className="relative">
            <Search
              size={16}
              className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
            />
            <input
              ref={inputRef}
              type="text"
              placeholder="Search workflows, routes, schemas, node types..."
              value={search}
              onChange={(e) => updateSearch(e.target.value)}
              onKeyDown={handleKeyDown}
              className="w-full pl-9 pr-3 py-2 text-sm border-none focus:outline-none"
            />
          </div>
        </div>

        <div ref={listRef} className="flex-1 overflow-y-auto py-1">
          {filtered.map((item, i) => (
            <button
              key={item.id}
              onClick={item.action}
              onMouseEnter={() => setSelectedIndex(i)}
              className={`w-full text-left px-4 py-2 flex items-center gap-3 ${
                i === selectedIndex
                  ? "bg-blue-50 text-blue-700"
                  : "text-gray-700 hover:bg-gray-50"
              }`}
            >
              <span className="shrink-0">{item.icon}</span>
              <span className="text-sm font-medium truncate">{item.label}</span>
              {item.detail && (
                <span className="text-xs text-gray-400 ml-auto shrink-0">
                  {item.detail}
                </span>
              )}
            </button>
          ))}
          {filtered.length === 0 && (
            <div className="p-4 text-sm text-gray-400 text-center">
              No results found.
            </div>
          )}
        </div>

        <div className="px-3 py-2 border-t border-gray-100 text-xs text-gray-400 flex gap-3">
          <span>
            <kbd className="px-1 py-0.5 bg-gray-100 rounded text-gray-500">
              ↑↓
            </kbd>{" "}
            Navigate
          </span>
          <span>
            <kbd className="px-1 py-0.5 bg-gray-100 rounded text-gray-500">
              ↵
            </kbd>{" "}
            Open
          </span>
          <span>
            <kbd className="px-1 py-0.5 bg-gray-100 rounded text-gray-500">
              Esc
            </kbd>{" "}
            Close
          </span>
        </div>
      </div>
    </div>
  );
}
