interface CRUDGenerationDialogProps {
  tableName: string;
  operations: string[];
  artifacts: string[];
  service: string;
  basePath: string;
  scopeCol: string;
  scopeParam: string;
  preview: Record<string, unknown> | null;
  onToggleOp: (op: string) => void;
  onToggleArtifact: (a: string) => void;
  onServiceChange: (s: string) => void;
  onBasePathChange: (p: string) => void;
  onScopeColChange: (c: string) => void;
  onScopeParamChange: (p: string) => void;
  onPreview: () => void;
  onConfirm: () => void;
  onClose: () => void;
}

export function CRUDGenerationDialog({
  tableName,
  operations,
  artifacts,
  service,
  basePath,
  scopeCol,
  scopeParam,
  preview,
  onToggleOp,
  onToggleArtifact,
  onServiceChange,
  onBasePathChange,
  onScopeColChange,
  onScopeParamChange,
  onPreview,
  onConfirm,
  onClose,
}: CRUDGenerationDialogProps) {
  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl w-[700px] max-h-[80vh] flex flex-col">
        <div className="px-6 py-4 border-b border-gray-200 flex items-center justify-between">
          <h3 className="text-lg font-semibold text-gray-900">
            Generate CRUD for {tableName}
          </h3>
          <button
            onClick={onClose}
            className="text-gray-400 hover:text-gray-600"
          >
            x
          </button>
        </div>
        <div className="flex-1 overflow-auto p-6 space-y-4">
          {!preview ? (
            <>
              <div>
                <h4 className="text-xs font-medium text-gray-500 uppercase mb-2">
                  Operations
                </h4>
                <div className="flex gap-3 flex-wrap">
                  {["create", "list", "get", "update", "delete"].map((op) => (
                    <label
                      key={op}
                      className="flex items-center gap-1.5 text-sm text-gray-700"
                    >
                      <input
                        type="checkbox"
                        checked={operations.includes(op)}
                        onChange={() => onToggleOp(op)}
                        className="rounded"
                      />
                      {op}
                    </label>
                  ))}
                </div>
              </div>

              <div>
                <h4 className="text-xs font-medium text-gray-500 uppercase mb-2">
                  Artifacts
                </h4>
                <div className="flex gap-3 flex-wrap">
                  {["routes", "workflows", "schemas"].map((a) => (
                    <label
                      key={a}
                      className="flex items-center gap-1.5 text-sm text-gray-700"
                    >
                      <input
                        type="checkbox"
                        checked={artifacts.includes(a)}
                        onChange={() => onToggleArtifact(a)}
                        className="rounded"
                      />
                      {a}
                    </label>
                  ))}
                </div>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="text-xs text-gray-500">Service</label>
                  <input
                    type="text"
                    value={service}
                    onChange={(e) => onServiceChange(e.target.value)}
                    className="w-full mt-1 px-2 py-1 text-sm border border-gray-300 rounded font-mono"
                  />
                </div>
                <div>
                  <label className="text-xs text-gray-500">Base Path</label>
                  <input
                    type="text"
                    value={basePath}
                    onChange={(e) => onBasePathChange(e.target.value)}
                    className="w-full mt-1 px-2 py-1 text-sm border border-gray-300 rounded font-mono"
                    placeholder={`/api/${tableName}`}
                  />
                </div>
                <div>
                  <label className="text-xs text-gray-500">
                    Scope Column (optional)
                  </label>
                  <input
                    type="text"
                    value={scopeCol}
                    onChange={(e) => onScopeColChange(e.target.value)}
                    className="w-full mt-1 px-2 py-1 text-sm border border-gray-300 rounded font-mono"
                    placeholder="e.g. workspace_id"
                  />
                </div>
                <div>
                  <label className="text-xs text-gray-500">
                    Scope Param (optional)
                  </label>
                  <input
                    type="text"
                    value={scopeParam}
                    onChange={(e) => onScopeParamChange(e.target.value)}
                    className="w-full mt-1 px-2 py-1 text-sm border border-gray-300 rounded font-mono"
                    placeholder="e.g. workspace_id"
                  />
                </div>
              </div>
            </>
          ) : (
            <div className="space-y-3">
              <h4 className="text-xs font-medium text-gray-500 uppercase">
                Files to create ({Object.keys(preview).length})
              </h4>
              {Object.entries(preview).map(([path, content]) => (
                <div key={path}>
                  <div className="text-xs font-mono text-blue-600 mb-1">
                    {path}
                  </div>
                  <pre className="p-2 bg-gray-50 rounded text-xs text-gray-700 overflow-x-auto border border-gray-200 max-h-32 whitespace-pre-wrap">
                    {JSON.stringify(content, null, 2)}
                  </pre>
                </div>
              ))}
            </div>
          )}
        </div>
        <div className="px-6 py-3 border-t border-gray-200 flex justify-end gap-2">
          <button
            onClick={onClose}
            className="px-3 py-1.5 text-sm text-gray-600 border border-gray-300 rounded hover:bg-gray-50"
          >
            Cancel
          </button>
          {!preview ? (
            <button
              onClick={onPreview}
              disabled={operations.length === 0}
              className="px-3 py-1.5 text-sm text-white bg-indigo-500 rounded hover:bg-indigo-600 disabled:opacity-40"
            >
              Preview
            </button>
          ) : (
            <button
              onClick={onConfirm}
              className="px-3 py-1.5 text-sm text-white bg-indigo-500 rounded hover:bg-indigo-600"
            >
              Create Files
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
