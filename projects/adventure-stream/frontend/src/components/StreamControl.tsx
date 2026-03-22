import { useState, useEffect, useCallback } from "react";
import {
  startStream,
  stopStream,
  getAdminStreamStatus,
} from "../api";
import type { AdminStreamStatus } from "../types";

export default function StreamControl() {
  const [status, setStatus] = useState<AdminStreamStatus | null>(null);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

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

  const isLive = !!status?.stream;

  const handleStart = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!title.trim()) return;
    setLoading(true);
    setError(null);
    try {
      await startStream(title.trim(), description.trim() || undefined);
      setTitle("");
      setDescription("");
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to start stream");
    } finally {
      setLoading(false);
    }
  };

  const handleStop = async () => {
    setLoading(true);
    setError(null);
    try {
      await stopStream();
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to stop stream");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="rounded-xl border border-gray-800 bg-gray-900 p-4 sm:p-6">
      <h3 className="mb-4 text-lg font-semibold text-white">
        Stream Control
      </h3>

      {error && (
        <div className="mb-4 rounded-lg bg-red-900/50 px-3 py-2 text-sm text-red-300">
          {error}
        </div>
      )}

      {isLive ? (
        <div>
          <div className="mb-4 flex items-center gap-2">
            <span className="inline-block h-3 w-3 animate-pulse rounded-full bg-green-500" />
            <span className="font-medium text-green-400">LIVE</span>
          </div>
          <div className="mb-4 space-y-1 text-sm">
            <p className="text-white">
              <span className="text-gray-400">Title:</span>{" "}
              {status?.stream?.title}
            </p>
            {status?.stream?.description && (
              <p className="text-white">
                <span className="text-gray-400">Description:</span>{" "}
                {status.stream.description}
              </p>
            )}
            <p className="text-gray-400">
              Started: {status?.stream?.started_at ? new Date(status.stream.started_at).toLocaleString() : ""}
            </p>
          </div>
          <button
            onClick={handleStop}
            disabled={loading}
            className="rounded-lg bg-red-600 px-6 py-2 font-medium text-white transition hover:bg-red-700 disabled:opacity-50"
          >
            {loading ? "Stopping..." : "Stop Stream"}
          </button>
        </div>
      ) : (
        <form onSubmit={handleStart} className="space-y-4">
          <div>
            <label className="mb-1 block text-sm text-gray-400">Title</label>
            <input
              type="text"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="Morning hike in the Alps"
              required
              className="w-full rounded-lg border border-gray-700 bg-gray-800 px-4 py-2 text-white placeholder-gray-500 focus:border-blue-500 focus:outline-none"
            />
          </div>
          <div>
            <label className="mb-1 block text-sm text-gray-400">
              Description (optional)
            </label>
            <textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Join me on a sunrise trail run..."
              rows={2}
              className="w-full rounded-lg border border-gray-700 bg-gray-800 px-4 py-2 text-white placeholder-gray-500 focus:border-blue-500 focus:outline-none"
            />
          </div>
          <button
            type="submit"
            disabled={loading || !title.trim()}
            className="rounded-lg bg-green-600 px-6 py-2 font-medium text-white transition hover:bg-green-700 disabled:opacity-50"
          >
            {loading ? "Starting..." : "Start Stream"}
          </button>
        </form>
      )}
    </div>
  );
}
