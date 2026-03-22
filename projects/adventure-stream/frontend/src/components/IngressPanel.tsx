import { useState, useEffect, useCallback } from "react";
import { createIngress, listIngress, deleteIngress } from "../api";
import type { IngressInfo } from "../types";

export default function IngressPanel() {
  const [items, setItems] = useState<IngressInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const res = await listIngress();
      setItems(res.items ?? []);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load ingress");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const handleCreate = async () => {
    setCreating(true);
    setError(null);
    try {
      await createIngress("DJI Osmo Action 4");
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to create ingress");
    } finally {
      setCreating(false);
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await deleteIngress(id);
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to delete ingress");
    }
  };

  const copyToClipboard = (text: string, label: string) => {
    navigator.clipboard.writeText(text);
    setCopied(label);
    setTimeout(() => setCopied(null), 2000);
  };

  return (
    <div className="rounded-xl border border-gray-800 bg-gray-900 p-4 sm:p-6">
      <div className="mb-4 flex items-center justify-between gap-2">
        <h3 className="text-lg font-semibold text-white">RTMP Ingress</h3>
        <button
          onClick={handleCreate}
          disabled={creating}
          className="shrink-0 rounded-lg bg-blue-600 px-3 py-1.5 text-sm font-medium text-white transition hover:bg-blue-700 disabled:opacity-50 sm:px-4 sm:py-2"
        >
          {creating ? "Creating..." : "Create Ingress"}
        </button>
      </div>
      <p className="mb-4 text-sm text-gray-400">
        Create an RTMP endpoint. Copy the combined URL into DJI Mimo's RTMP
        address field.
      </p>

      {error && (
        <div className="mb-4 rounded-lg bg-red-900/50 px-3 py-2 text-sm text-red-300">
          {error}
        </div>
      )}

      {loading ? (
        <p className="text-sm text-gray-500">Loading...</p>
      ) : items.length === 0 ? (
        <p className="text-sm text-gray-500">
          No ingress endpoints. Create one to get your RTMP URL.
        </p>
      ) : (
        <div className="space-y-4">
          {items.map((item) => (
            <div
              key={item.ingress_id}
              className="rounded-lg border border-gray-700 bg-gray-800 p-4"
            >
              <div className="mb-3 flex items-center justify-between">
                <span className="text-sm font-medium text-gray-300">
                  {item.participant_name}
                </span>
                <button
                  onClick={() => handleDelete(item.ingress_id)}
                  className="text-sm text-red-400 transition hover:text-red-300"
                >
                  Delete
                </button>
              </div>
              <div className="space-y-3">
                <div>
                  <label className="mb-1 block text-xs font-medium text-blue-400">
                    DJI Mimo RTMP Address (copy this)
                  </label>
                  <div className="overflow-x-auto rounded bg-gray-900 px-3 py-1.5">
                    <code className="whitespace-nowrap text-sm text-blue-300">
                      {item.url}/{item.stream_key}
                    </code>
                  </div>
                  <button
                    onClick={() =>
                      copyToClipboard(
                        `${item.url}/${item.stream_key}`,
                        `mimo-${item.ingress_id}`,
                      )
                    }
                    className="mt-2 w-full rounded bg-blue-700 px-3 py-2 text-sm font-medium text-white transition hover:bg-blue-600 sm:w-auto"
                  >
                    {copied === `mimo-${item.ingress_id}`
                      ? "Copied!"
                      : "Copy RTMP Address"}
                  </button>
                </div>
                <details className="text-xs text-gray-500">
                  <summary className="cursor-pointer hover:text-gray-400">
                    Show URL and Stream Key separately
                  </summary>
                  <div className="mt-2 space-y-1.5">
                    <div className="flex items-center gap-2">
                      <span className="w-16 shrink-0 text-gray-500">URL</span>
                      <code className="flex-1 truncate rounded bg-gray-900 px-2 py-1 text-green-400">
                        {item.url}
                      </code>
                      <button
                        onClick={() =>
                          copyToClipboard(item.url, `url-${item.ingress_id}`)
                        }
                        className="rounded bg-gray-700 px-2 py-1 text-gray-300 transition hover:bg-gray-600"
                      >
                        {copied === `url-${item.ingress_id}`
                          ? "Copied!"
                          : "Copy"}
                      </button>
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="w-16 shrink-0 text-gray-500">Key</span>
                      <code className="flex-1 truncate rounded bg-gray-900 px-2 py-1 text-yellow-400">
                        {item.stream_key}
                      </code>
                      <button
                        onClick={() =>
                          copyToClipboard(
                            item.stream_key,
                            `key-${item.ingress_id}`,
                          )
                        }
                        className="rounded bg-gray-700 px-2 py-1 text-gray-300 transition hover:bg-gray-600"
                      >
                        {copied === `key-${item.ingress_id}`
                          ? "Copied!"
                          : "Copy"}
                      </button>
                    </div>
                  </div>
                </details>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
