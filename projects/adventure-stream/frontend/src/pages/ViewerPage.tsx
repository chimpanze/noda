import { useState, useEffect, useCallback } from "react";
import { getStreamStatus, getStreamToken } from "../api";
import type { StreamStatus, StreamToken } from "../types";
import OfflineState from "../components/OfflineState";
import ViewerJoin from "../components/ViewerJoin";
import StreamPlayer from "../components/StreamPlayer";

export default function ViewerPage() {
  const [status, setStatus] = useState<StreamStatus | null>(null);
  const [connection, setConnection] = useState<StreamToken | null>(null);
  const [joining, setJoining] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const pollStatus = useCallback(async () => {
    try {
      const s = await getStreamStatus();
      setStatus(s);
      if (!s.live && connection) {
        setConnection(null);
      }
    } catch {
      // silently retry
    }
  }, [connection]);

  useEffect(() => {
    pollStatus();
    const id = setInterval(pollStatus, 30000);
    return () => clearInterval(id);
  }, [pollStatus]);

  const handleJoin = async (name: string) => {
    setJoining(true);
    setError(null);
    try {
      const token = await getStreamToken(name);
      setConnection(token);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to join");
    } finally {
      setJoining(false);
    }
  };

  return (
    <div className="flex min-h-screen flex-col bg-gray-950 text-white">
      <header className="flex items-center justify-between border-b border-gray-800 px-6 py-4">
        <h1 className="text-xl font-bold">Adventure Stream</h1>
        <div className="flex items-center gap-4">
          {status?.live && (
            <span className="flex items-center gap-1.5 text-sm font-medium text-green-400">
              <span className="inline-block h-2 w-2 animate-pulse rounded-full bg-green-400" />
              LIVE
            </span>
          )}
        </div>
      </header>

      <main className="flex flex-1 flex-col">
        {error && (
          <div className="mx-auto mb-4 rounded-lg bg-red-900/50 px-4 py-2 text-sm text-red-300">
            {error}
          </div>
        )}

        {!status || !status.live ? (
          <div className="flex flex-1 items-center justify-center">
            <OfflineState />
          </div>
        ) : connection ? (
          <div className="h-[calc(100vh-57px)] w-full bg-black">
            <StreamPlayer
              token={connection.token}
              serverUrl={connection.url}
              onDisconnected={() => setConnection(null)}
            />
          </div>
        ) : (
          <div className="flex flex-1 items-center justify-center">
            <ViewerJoin
              status={status}
              onJoin={handleJoin}
              joining={joining}
            />
          </div>
        )}
      </main>
    </div>
  );
}
