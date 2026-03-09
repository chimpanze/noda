import { useEffect, useState } from "react";
import { X, AlertCircle, CheckCircle } from "lucide-react";

export interface ToastMessage {
  id: string;
  type: "error" | "success" | "info";
  message: string;
  action?: { label: string; onClick: () => void };
}

let toastId = 0;
const listeners: Set<(msg: ToastMessage) => void> = new Set();

export function showToast(msg: Omit<ToastMessage, "id">) {
  const toast: ToastMessage = { ...msg, id: String(++toastId) };
  listeners.forEach((fn) => fn(toast));
}

export function ToastContainer() {
  const [toasts, setToasts] = useState<ToastMessage[]>([]);

  useEffect(() => {
    const handler = (msg: ToastMessage) => {
      setToasts((prev) => [...prev.slice(-4), msg]);
      setTimeout(() => {
        setToasts((prev) => prev.filter((t) => t.id !== msg.id));
      }, 5000);
    };
    listeners.add(handler);
    return () => { listeners.delete(handler); };
  }, []);

  const dismiss = (id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  };

  if (toasts.length === 0) return null;

  return (
    <div className="fixed bottom-4 right-4 z-50 space-y-2">
      {toasts.map((toast) => (
        <div
          key={toast.id}
          className={`flex items-start gap-2 px-4 py-3 rounded-lg shadow-lg text-sm max-w-sm ${
            toast.type === "error"
              ? "bg-red-50 border border-red-200 text-red-800"
              : toast.type === "success"
                ? "bg-green-50 border border-green-200 text-green-800"
                : "bg-blue-50 border border-blue-200 text-blue-800"
          }`}
        >
          {toast.type === "error" ? (
            <AlertCircle size={16} className="shrink-0 mt-0.5" />
          ) : (
            <CheckCircle size={16} className="shrink-0 mt-0.5" />
          )}
          <div className="flex-1">
            <span>{toast.message}</span>
            {toast.action && (
              <button
                onClick={toast.action.onClick}
                className="ml-2 underline font-medium hover:no-underline"
              >
                {toast.action.label}
              </button>
            )}
          </div>
          <button onClick={() => dismiss(toast.id)} className="shrink-0 opacity-60 hover:opacity-100">
            <X size={14} />
          </button>
        </div>
      ))}
    </div>
  );
}
