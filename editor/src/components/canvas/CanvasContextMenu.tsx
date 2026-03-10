import { useEffect, useRef } from "react";
import { Copy, Trash2, Clipboard, LayoutGrid, Plus, RotateCcw } from "lucide-react";

export type ContextMenuType = "node" | "edge" | "pane";

export interface ContextMenuState {
  type: ContextMenuType;
  x: number;
  y: number;
  targetId?: string; // node id or edge id
}

interface ContextMenuProps {
  menu: ContextMenuState;
  onClose: () => void;
  // Node actions
  onDuplicateNode?: () => void;
  onDeleteNode?: () => void;
  onCopyNode?: () => void;
  // Edge actions
  onToggleRetry?: () => void;
  onDeleteEdge?: () => void;
  hasRetry?: boolean;
  // Pane actions
  onAddNode?: () => void;
  onPaste?: () => void;
  onAutoLayout?: () => void;
  canPaste?: boolean;
}

export function CanvasContextMenu({
  menu,
  onClose,
  onDuplicateNode,
  onDeleteNode,
  onCopyNode,
  onToggleRetry,
  onDeleteEdge,
  hasRetry,
  onAddNode,
  onPaste,
  onAutoLayout,
  canPaste,
}: ContextMenuProps) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Element)) {
        onClose();
      }
    };
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("mousedown", handleClick);
    document.addEventListener("keydown", handleKey);
    return () => {
      document.removeEventListener("mousedown", handleClick);
      document.removeEventListener("keydown", handleKey);
    };
  }, [onClose]);

  const items: { label: string; icon: React.ReactNode; onClick: () => void; danger?: boolean }[] = [];

  if (menu.type === "node") {
    items.push(
      { label: "Copy", icon: <Copy size={14} />, onClick: () => { onCopyNode?.(); onClose(); } },
      { label: "Duplicate", icon: <Clipboard size={14} />, onClick: () => { onDuplicateNode?.(); onClose(); } },
      { label: "Delete", icon: <Trash2 size={14} />, onClick: () => { onDeleteNode?.(); onClose(); }, danger: true },
    );
  } else if (menu.type === "edge") {
    items.push(
      {
        label: hasRetry ? "Remove Retry" : "Add Retry",
        icon: <RotateCcw size={14} />,
        onClick: () => { onToggleRetry?.(); onClose(); },
      },
      { label: "Delete Edge", icon: <Trash2 size={14} />, onClick: () => { onDeleteEdge?.(); onClose(); }, danger: true },
    );
  } else {
    items.push(
      { label: "Add Node", icon: <Plus size={14} />, onClick: () => { onAddNode?.(); onClose(); } },
    );
    if (canPaste) {
      items.push(
        { label: "Paste", icon: <Clipboard size={14} />, onClick: () => { onPaste?.(); onClose(); } },
      );
    }
    items.push(
      { label: "Auto Layout", icon: <LayoutGrid size={14} />, onClick: () => { onAutoLayout?.(); onClose(); } },
    );
  }

  return (
    <div
      ref={ref}
      className="fixed z-50 bg-white border border-gray-200 rounded-lg shadow-lg py-1 min-w-[160px]"
      style={{ left: menu.x, top: menu.y }}
    >
      {items.map((item, i) => (
        <button
          key={i}
          onClick={item.onClick}
          className={`w-full text-left px-3 py-1.5 text-sm flex items-center gap-2 hover:bg-gray-50 ${
            item.danger ? "text-red-600 hover:bg-red-50" : "text-gray-700"
          }`}
        >
          {item.icon}
          {item.label}
        </button>
      ))}
    </div>
  );
}
