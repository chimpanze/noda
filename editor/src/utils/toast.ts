export interface ToastMessage {
  id: string;
  type: "error" | "success" | "info";
  message: string;
  action?: { label: string; onClick: () => void };
}

let toastId = 0;
export const listeners: Set<(msg: ToastMessage) => void> = new Set();

export function showToast(msg: Omit<ToastMessage, "id">) {
  const toast: ToastMessage = { ...msg, id: String(++toastId) };
  listeners.forEach((fn) => fn(toast));
}
