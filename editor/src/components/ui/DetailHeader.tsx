import { Trash2 } from "lucide-react";

export function DetailHeader({
  title,
  isNew,
  saving,
  onSave,
  onDelete,
  saveDisabled,
}: {
  title: string;
  isNew?: boolean;
  saving?: boolean;
  onSave: () => void;
  onDelete?: () => void;
  saveDisabled?: boolean;
}) {
  return (
    <div className="flex items-center justify-between mb-6">
      <h3 className="text-lg font-semibold text-gray-900">{title}</h3>
      <div className="flex items-center gap-2">
        {!isNew && onDelete && (
          <button
            onClick={onDelete}
            className="px-3 py-1.5 text-sm text-red-600 border border-red-300 rounded hover:bg-red-50"
          >
            <Trash2 size={14} className="inline mr-1" />
            Delete
          </button>
        )}
        <button
          onClick={onSave}
          disabled={saving || saveDisabled}
          className="px-4 py-1.5 text-sm text-white bg-blue-500 rounded hover:bg-blue-600 disabled:opacity-50"
        >
          {saving ? "Saving..." : "Save"}
        </button>
      </div>
    </div>
  );
}
