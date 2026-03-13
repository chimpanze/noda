import { useState, useMemo, useCallback } from "react";
import { Search, GripVertical, BookOpen } from "lucide-react";
import { useEditorStore } from "@/stores/editor";
import { nodeDocIndex } from "virtual:docs";
import { getCategoryStyle } from "./nodeStyles";

export function NodePalette() {
  const nodeTypes = useEditorStore((s) => s.nodeTypes);
  const openDoc = useEditorStore((s) => s.openDoc);
  const [search, setSearch] = useState("");
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());

  // Group node types by prefix
  const groups = useMemo(() => {
    const map = new Map<string, typeof nodeTypes>();
    for (const nt of nodeTypes) {
      const prefix = nt.type.split(".")[0];
      if (!map.has(prefix)) map.set(prefix, []);
      map.get(prefix)!.push(nt);
    }
    // Sort groups by prefix
    return Array.from(map.entries()).sort(([a], [b]) => a.localeCompare(b));
  }, [nodeTypes]);

  // Filter by search
  const filtered = useMemo(() => {
    if (!search) return groups;
    const lower = search.toLowerCase();
    return groups
      .map(([prefix, nodes]) => {
        const matching = nodes.filter(
          (n) =>
            n.type.toLowerCase().includes(lower) ||
            n.name.toLowerCase().includes(lower) ||
            (n.description?.toLowerCase().includes(lower) ?? false),
        );
        return [prefix, matching] as [string, typeof nodeTypes];
      })
      .filter(([, nodes]) => nodes.length > 0);
  }, [groups, search]);

  const toggleGroup = useCallback((prefix: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(prefix)) next.delete(prefix);
      else next.add(prefix);
      return next;
    });
  }, []);

  const onDragStart = useCallback(
    (event: React.DragEvent, nodeType: string) => {
      event.dataTransfer.setData("application/noda-node-type", nodeType);
      event.dataTransfer.effectAllowed = "move";
    },
    [],
  );

  return (
    <div className="w-52 border-r border-gray-200 flex flex-col bg-white shrink-0 overflow-hidden">
      <div className="p-2 border-b border-gray-200">
        <div className="relative">
          <Search
            size={14}
            className="absolute left-2 top-1/2 -translate-y-1/2 text-gray-400"
          />
          <input
            type="text"
            placeholder="Search nodes..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full pl-7 pr-2 py-1.5 text-xs border border-gray-200 rounded focus:outline-none focus:ring-1 focus:ring-blue-400"
          />
        </div>
      </div>

      <div className="flex-1 overflow-y-auto py-1">
        {filtered.map(([prefix, nodes]) => {
          const style = getCategoryStyle(prefix);
          const isCollapsed = collapsed.has(prefix);

          return (
            <div key={prefix}>
              <button
                onClick={() => toggleGroup(prefix)}
                className="w-full flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-gray-600 hover:bg-gray-50"
              >
                <span
                  className={`w-2 h-2 rounded-full ${style.bg} ${style.border} border`}
                />
                <span className="flex-1 text-left capitalize">{prefix}</span>
                <span className="text-gray-400">{isCollapsed ? "+" : "−"}</span>
              </button>
              {!isCollapsed &&
                nodes.map((nt) => (
                  <div
                    key={nt.type}
                    draggable
                    onDragStart={(e) => onDragStart(e, nt.type)}
                    title={nt.description}
                    className="group flex items-center gap-1.5 px-4 py-1 cursor-grab active:cursor-grabbing hover:bg-blue-50 text-xs text-gray-700"
                  >
                    <GripVertical
                      size={10}
                      className="text-gray-300 shrink-0"
                    />
                    <span className="truncate flex-1">{nt.type}</span>
                    {nodeDocIndex[nt.type] && (
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          e.preventDefault();
                          openDoc(nodeDocIndex[nt.type]);
                        }}
                        className="opacity-0 group-hover:opacity-100 text-gray-400 hover:text-blue-500 shrink-0"
                        title="View docs"
                      >
                        <BookOpen size={10} />
                      </button>
                    )}
                  </div>
                ))}
            </div>
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
