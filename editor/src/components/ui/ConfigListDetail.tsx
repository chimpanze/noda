import { Plus } from "lucide-react";

interface ConfigListDetailProps<T> {
  items: T[];
  renderItem: (item: T, selected: boolean) => React.ReactNode;
  getKey: (item: T) => string;
  selectedKey: string | null;
  onSelect: (key: string) => void;
  title: string;
  onNew?: () => void;
  sidebarWidth?: string;
  emptyMessage?: string;
  filter?: {
    value: string;
    onChange: (v: string) => void;
    placeholder: string;
  };
  sidebarExtra?: React.ReactNode;
  children: React.ReactNode;
}

export function ConfigListDetail<T>({
  items,
  renderItem,
  getKey,
  selectedKey,
  onSelect,
  title,
  onNew,
  sidebarWidth = "w-80",
  emptyMessage,
  filter,
  sidebarExtra,
  children,
}: ConfigListDetailProps<T>) {
  return (
    <div className="flex-1 flex min-h-0">
      {/* Sidebar */}
      <div
        className={`${sidebarWidth} border-r border-gray-200 overflow-y-auto`}
      >
        <div className="px-4 py-3 border-b border-gray-200 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-gray-800">{title}</h2>
          {onNew && (
            <button
              onClick={onNew}
              className="flex items-center gap-1 px-2 py-1 text-xs text-blue-600 hover:bg-blue-50 rounded"
            >
              <Plus size={14} />
              New
            </button>
          )}
        </div>
        {filter && (
          <div className="px-4 py-2 border-b border-gray-100">
            <input
              type="text"
              value={filter.value}
              onChange={(e) => filter.onChange(e.target.value)}
              className="input-field text-sm"
              placeholder={filter.placeholder}
            />
          </div>
        )}
        {sidebarExtra}
        <div className="divide-y divide-gray-100">
          {items.map((item) => {
            const key = getKey(item);
            return (
              <button
                key={key}
                onClick={() => onSelect(key)}
                className={`w-full text-left px-4 py-2.5 hover:bg-gray-50 ${
                  selectedKey === key ? "bg-blue-50" : ""
                }`}
              >
                {renderItem(item, selectedKey === key)}
              </button>
            );
          })}
          {items.length === 0 && (
            <div className="p-4 text-sm text-gray-400">
              {emptyMessage ?? "No items."}
            </div>
          )}
        </div>
      </div>

      {/* Detail panel */}
      <div className="flex-1 overflow-y-auto p-6">{children}</div>
    </div>
  );
}
