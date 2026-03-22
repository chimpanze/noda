import { useState, useEffect, useCallback } from "react";
import { listParticipants, kickParticipant } from "../api";
import type { Participant } from "../types";

export default function ParticipantList() {
  const [participants, setParticipants] = useState<Participant[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const res = await listParticipants();
      setParticipants(res.participants ?? []);
    } catch {
      // room might not exist yet
      setParticipants([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
    const id = setInterval(refresh, 5000);
    return () => clearInterval(id);
  }, [refresh]);

  const handleKick = async (identity: string) => {
    setError(null);
    try {
      await kickParticipant(identity);
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to kick participant");
    }
  };

  return (
    <div className="rounded-xl border border-gray-800 bg-gray-900 p-4 sm:p-6">
      <div className="mb-4 flex items-center justify-between">
        <h3 className="text-lg font-semibold text-white">
          Participants ({participants.length})
        </h3>
        <button
          onClick={refresh}
          disabled={loading}
          className="text-sm text-gray-400 transition hover:text-white"
        >
          Refresh
        </button>
      </div>

      {error && (
        <div className="mb-4 rounded-lg bg-red-900/50 px-3 py-2 text-sm text-red-300">
          {error}
        </div>
      )}

      {participants.length === 0 ? (
        <p className="text-sm text-gray-500">No viewers connected.</p>
      ) : (
        <>
          {/* Mobile: card layout */}
          <div className="space-y-3 sm:hidden">
            {participants.map((p) => (
              <div
                key={p.sid}
                className="rounded-lg border border-gray-700 bg-gray-800 p-3"
              >
                <div className="flex items-center justify-between">
                  <span className="font-medium text-white">
                    {p.name || p.identity}
                  </span>
                  <span
                    className={`rounded px-2 py-0.5 text-xs ${
                      p.state === "ACTIVE"
                        ? "bg-green-900/50 text-green-400"
                        : "bg-gray-700 text-gray-400"
                    }`}
                  >
                    {p.state}
                  </span>
                </div>
                <div className="mt-1 text-xs text-gray-400">
                  Joined {new Date(p.joined_at * 1000).toLocaleTimeString()}
                </div>
                {p.identity !== "camera" && (
                  <button
                    onClick={() => handleKick(p.identity)}
                    className="mt-2 text-xs text-red-400 transition hover:text-red-300"
                  >
                    Kick
                  </button>
                )}
              </div>
            ))}
          </div>

          {/* Desktop: table layout */}
          <div className="hidden overflow-x-auto sm:block">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-gray-700 text-gray-400">
                  <th className="pb-2 pr-4">Name</th>
                  <th className="pb-2 pr-4">Identity</th>
                  <th className="pb-2 pr-4">State</th>
                  <th className="pb-2 pr-4">Joined</th>
                  <th className="pb-2" />
                </tr>
              </thead>
              <tbody>
                {participants.map((p) => (
                  <tr key={p.sid} className="border-b border-gray-800">
                    <td className="py-2 pr-4 text-white">
                      {p.name || p.identity}
                    </td>
                    <td className="py-2 pr-4 font-mono text-xs text-gray-400">
                      {p.identity}
                    </td>
                    <td className="py-2 pr-4">
                      <span
                        className={`inline-block rounded px-2 py-0.5 text-xs ${
                          p.state === "ACTIVE"
                            ? "bg-green-900/50 text-green-400"
                            : "bg-gray-700 text-gray-400"
                        }`}
                      >
                        {p.state}
                      </span>
                    </td>
                    <td className="py-2 pr-4 text-gray-400">
                      {new Date(p.joined_at * 1000).toLocaleTimeString()}
                    </td>
                    <td className="py-2">
                      {p.identity !== "camera" && (
                        <button
                          onClick={() => handleKick(p.identity)}
                          className="text-xs text-red-400 transition hover:text-red-300"
                        >
                          Kick
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  );
}
