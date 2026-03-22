import type {
  StreamStatus,
  StreamToken,
  IngressInfo,
  LoginResponse,
  AdminStreamStatus,
  Participant,
} from "./types";

const BASE = "/api";

function getToken(): string | null {
  return localStorage.getItem("admin_token");
}

export function setToken(token: string) {
  localStorage.setItem("admin_token", token);
}

export function clearToken() {
  localStorage.removeItem("admin_token");
}

export function hasToken(): boolean {
  return !!getToken();
}

async function request<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const token = getToken();
  const headers: Record<string, string> = {
    ...(options.headers as Record<string, string>),
  };
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
  if (options.body && typeof options.body === "string") {
    headers["Content-Type"] = "application/json";
  }

  const res = await fetch(`${BASE}${path}`, { ...options, headers });

  if (res.status === 401) {
    clearToken();
    throw new Error("Unauthorized");
  }
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: { message: res.statusText } }));
    throw new Error(err.error?.message ?? res.statusText);
  }
  return res.json() as Promise<T>;
}

// Public
export const getStreamStatus = () => request<StreamStatus>("/stream/status");

export const getStreamToken = (name: string) =>
  request<StreamToken>(`/stream/token?name=${encodeURIComponent(name)}`);

// Auth
export const login = (password: string) =>
  request<LoginResponse>("/auth/login", {
    method: "POST",
    body: JSON.stringify({ password }),
  });

// Admin - Ingress
export const createIngress = (participantName?: string) =>
  request<IngressInfo>("/admin/ingress", {
    method: "POST",
    body: JSON.stringify({ participant_name: participantName }),
  });

export const listIngress = () =>
  request<{ items: IngressInfo[] }>("/admin/ingress");

export const deleteIngress = (id: string) =>
  request<{ deleted: boolean }>(`/admin/ingress/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });

// Admin - Stream
export const startStream = (title: string, description?: string) =>
  request<{ room: object; title: string; description: string }>(
    "/admin/stream/start",
    {
      method: "POST",
      body: JSON.stringify({ title, description }),
    },
  );

export const stopStream = () =>
  request<{ stopped: boolean }>("/admin/stream/stop", { method: "POST" });

export const getAdminStreamStatus = () =>
  request<AdminStreamStatus>("/admin/stream/status");

// Admin - Participants
export const listParticipants = () =>
  request<{ participants: Participant[] }>("/admin/participants");

export const kickParticipant = (identity: string) =>
  request<{ removed: boolean }>(
    `/admin/participants/${encodeURIComponent(identity)}`,
    { method: "DELETE" },
  );
