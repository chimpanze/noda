export function MigrationPreviewDialog({
  migrationUp,
  migrationDown,
  onClose,
  onConfirm,
}: {
  migrationUp: string;
  migrationDown: string;
  onClose: () => void;
  onConfirm: () => void;
}) {
  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl w-[700px] max-h-[80vh] flex flex-col">
        <div className="px-6 py-4 border-b border-gray-200 flex items-center justify-between">
          <h3 className="text-lg font-semibold text-gray-900">
            Migration Preview
          </h3>
          <button
            onClick={onClose}
            className="text-gray-400 hover:text-gray-600"
          >
            x
          </button>
        </div>
        <div className="flex-1 overflow-auto p-6 space-y-4">
          <div>
            <h4 className="text-xs font-medium text-gray-500 uppercase mb-1">
              Up Migration
            </h4>
            <pre className="p-3 bg-gray-50 rounded text-xs text-gray-700 overflow-x-auto border border-gray-200 whitespace-pre-wrap">
              {migrationUp}
            </pre>
          </div>
          <div>
            <h4 className="text-xs font-medium text-gray-500 uppercase mb-1">
              Down Migration
            </h4>
            <pre className="p-3 bg-gray-50 rounded text-xs text-gray-700 overflow-x-auto border border-gray-200 whitespace-pre-wrap">
              {migrationDown}
            </pre>
          </div>
        </div>
        <div className="px-6 py-3 border-t border-gray-200 flex justify-end gap-2">
          <button
            onClick={onClose}
            className="px-3 py-1.5 text-sm text-gray-600 border border-gray-300 rounded hover:bg-gray-50"
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            className="px-3 py-1.5 text-sm text-white bg-indigo-500 rounded hover:bg-indigo-600"
          >
            Create Migration Files
          </button>
        </div>
      </div>
    </div>
  );
}
