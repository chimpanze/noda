import { WifiOff, RefreshCw } from "lucide-react";
import { useTraceStore } from "@/stores/trace";
import { connectTrace } from "@/api/traceClient";

export function ConnectionOverlay() {
  const status = useTraceStore((s) => s.connectionStatus);

  if (status !== "disconnected") return null;

  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-black/20 pointer-events-none">
      <div className="bg-white rounded-lg shadow-xl p-6 text-center max-w-xs pointer-events-auto">
        <WifiOff size={32} className="mx-auto text-gray-400 mb-3" />
        <h3 className="text-sm font-semibold text-gray-800 mb-1">Connection Lost</h3>
        <p className="text-xs text-gray-500 mb-4">
          Lost connection to the Noda backend. Live tracing is unavailable.
        </p>
        <button
          onClick={() => connectTrace()}
          className="flex items-center gap-1.5 mx-auto px-4 py-2 text-sm bg-blue-600 text-white rounded hover:bg-blue-700"
        >
          <RefreshCw size={14} />
          Reconnect
        </button>
      </div>
    </div>
  );
}
