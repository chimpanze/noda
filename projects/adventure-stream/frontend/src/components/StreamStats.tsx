import { useState, useEffect, useCallback } from "react";
import { getAdminStreamStatus } from "../api";
import type { AdminStreamStatus } from "../types";

export default function StreamStats() {
  const [status, setStatus] = useState<AdminStreamStatus | null>(null);

  const refresh = useCallback(async () => {
    try {
      const s = await getAdminStreamStatus();
      setStatus(s);
    } catch {
      // ignore
    }
  }, []);

  useEffect(() => {
    refresh();
    const id = setInterval(refresh, 5000);
    return () => clearInterval(id);
  }, [refresh]);

  const room = status?.rooms?.[0];

  return (
    <div className="rounded-xl border border-gray-800 bg-gray-900 p-4 sm:p-6">
      <h3 className="mb-4 text-lg font-semibold text-white">Stream Stats</h3>
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <Stat
          label="Status"
          value={status?.stream ? "Live" : "Offline"}
          color={status?.stream ? "text-green-400" : "text-gray-400"}
        />
        <Stat
          label="Viewers"
          value={room ? String(room.num_participants) : "0"}
        />
        <Stat
          label="Ingress"
          value={String(status?.ingress?.length ?? 0)}
        />
        <Stat
          label="Room SID"
          value={room?.sid?.slice(0, 12) ?? "—"}
          mono
        />
      </div>
    </div>
  );
}

function Stat({
  label,
  value,
  color,
  mono,
}: {
  label: string;
  value: string;
  color?: string;
  mono?: boolean;
}) {
  return (
    <div>
      <p className="text-xs text-gray-500">{label}</p>
      <p
        className={`text-lg font-semibold ${color ?? "text-white"} ${mono ? "font-mono text-sm" : ""}`}
      >
        {value}
      </p>
    </div>
  );
}
