import { X } from "lucide-react";

const shortcuts = [
  { keys: "Ctrl+Z", action: "Undo" },
  { keys: "Ctrl+Shift+Z / Ctrl+Y", action: "Redo" },
  { keys: "Ctrl+S", action: "Save workflow" },
  { keys: "Ctrl+C", action: "Copy selected node" },
  { keys: "Ctrl+V", action: "Paste nodes" },
  { keys: "Ctrl+A", action: "Select all nodes" },
  { keys: "Ctrl+Shift+F", action: "Auto-layout" },
  { keys: "Delete / Backspace", action: "Remove selected node" },
  { keys: "Escape", action: "Deselect all" },
  { keys: "?", action: "Show this help" },
];

export function ShortcutModal({ onClose }: { onClose: () => void }) {
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      onClick={onClose}
    >
      <div
        className="bg-white rounded-lg shadow-xl w-96 max-h-[80vh] overflow-y-auto"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between px-4 py-3 border-b border-gray-200">
          <h2 className="text-sm font-semibold text-gray-900">
            Keyboard Shortcuts
          </h2>
          <button
            onClick={onClose}
            className="text-gray-400 hover:text-gray-600"
          >
            <X size={16} />
          </button>
        </div>
        <div className="p-4 space-y-2">
          {shortcuts.map(({ keys, action }) => (
            <div
              key={keys}
              className="flex items-center justify-between text-sm"
            >
              <span className="text-gray-600">{action}</span>
              <kbd className="px-2 py-0.5 bg-gray-100 border border-gray-200 rounded text-xs font-mono text-gray-700">
                {keys}
              </kbd>
            </div>
          ))}
        </div>
        <div className="px-4 py-3 border-t border-gray-100 text-xs text-gray-400">
          On macOS, use Cmd instead of Ctrl.
        </div>
      </div>
    </div>
  );
}
