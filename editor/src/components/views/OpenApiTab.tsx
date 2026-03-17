import { Copy, Download } from "lucide-react";
import Editor from "@monaco-editor/react";
import { showToast } from "@/utils/toast";

export function OpenApiTab({
  spec,
  loading,
}: {
  spec: string | null;
  loading: boolean;
}) {
  return (
    <div className="flex-1 overflow-y-auto p-6">
      <div className="max-w-4xl space-y-3">
        <div className="flex items-center justify-between">
          <h3 className="text-lg font-semibold text-gray-900">
            OpenAPI Specification
          </h3>
          {spec && (
            <div className="flex items-center gap-2">
              <button
                onClick={() => {
                  navigator.clipboard.writeText(spec);
                  showToast({
                    type: "success",
                    message: "Copied to clipboard",
                  });
                }}
                className="flex items-center gap-1 px-2 py-1 text-xs text-gray-500 hover:text-gray-700 hover:bg-gray-100 rounded"
              >
                <Copy size={12} />
                Copy
              </button>
              <button
                onClick={() => {
                  const blob = new Blob([spec], {
                    type: "application/json",
                  });
                  const url = URL.createObjectURL(blob);
                  const a = document.createElement("a");
                  a.href = url;
                  a.download = "openapi.json";
                  a.click();
                  URL.revokeObjectURL(url);
                }}
                className="flex items-center gap-1 px-2 py-1 text-xs text-blue-500 hover:text-blue-700 hover:bg-blue-50 rounded"
              >
                <Download size={12} />
                Download
              </button>
            </div>
          )}
        </div>
        {loading ? (
          <div className="text-sm text-gray-400">
            Generating OpenAPI spec...
          </div>
        ) : spec ? (
          <div className="border border-gray-200 rounded overflow-hidden">
            <Editor
              height="600px"
              language="json"
              value={spec}
              options={{
                minimap: { enabled: false },
                fontSize: 13,
                scrollBeyondLastLine: false,
                lineNumbers: "on",
                readOnly: true,
                wordWrap: "on",
              }}
            />
          </div>
        ) : (
          <div className="text-sm text-gray-400">
            Failed to load OpenAPI spec.
          </div>
        )}
      </div>
    </div>
  );
}
