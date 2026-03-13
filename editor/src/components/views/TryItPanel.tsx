import { useState, useCallback } from "react";
import { Play, Loader2 } from "lucide-react";
import Editor from "@monaco-editor/react";
import type { RouteConfig } from "./RouteFormPanel";

interface TryItPanelProps {
  route: RouteConfig;
}

interface TryItResponse {
  status: number;
  statusText: string;
  headers: Record<string, string>;
  body: string;
  duration: number;
  traceId?: string;
}

export function TryItPanel({ route }: TryItPanelProps) {
  const [headers, setHeaders] = useState<string>(
    '{\n  "Content-Type": "application/json"\n}',
  );
  const [body, setBody] = useState<string>("{}");
  const [response, setResponse] = useState<TryItResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const hasBody = ["POST", "PUT", "PATCH"].includes(route.method);

  const sendRequest = useCallback(async () => {
    setLoading(true);
    setError(null);
    setResponse(null);

    const startTime = performance.now();

    try {
      // Parse headers
      let parsedHeaders: Record<string, string> = {};
      try {
        parsedHeaders = JSON.parse(headers);
      } catch {
        setError("Invalid headers JSON");
        setLoading(false);
        return;
      }

      // Build request options
      const opts: RequestInit = {
        method: route.method,
        headers: parsedHeaders,
      };

      if (hasBody && body.trim()) {
        opts.body = body;
      }

      // Determine URL - use the route path, replace path params with placeholders
      const url = route.path.replace(/:(\w+)/g, (_, p) => `{${p}}`);

      const resp = await fetch(url, opts);
      const duration = Math.round(performance.now() - startTime);

      // Read response body
      const contentType = resp.headers.get("content-type") ?? "";
      let respBody: string;
      if (contentType.includes("json")) {
        const json = await resp.json();
        respBody = JSON.stringify(json, null, 2);
      } else {
        respBody = await resp.text();
      }

      // Extract trace ID if present
      const traceId =
        resp.headers.get("x-trace-id") ??
        (contentType.includes("json")
          ? (() => {
              try {
                const j = JSON.parse(respBody);
                return j.trace_id ?? j.error?.trace_id ?? undefined;
              } catch {
                return undefined;
              }
            })()
          : undefined);

      // Collect response headers
      const respHeaders: Record<string, string> = {};
      resp.headers.forEach((val, key) => {
        respHeaders[key] = val;
      });

      setResponse({
        status: resp.status,
        statusText: resp.statusText,
        headers: respHeaders,
        body: respBody,
        duration,
        traceId,
      });
    } catch (err) {
      setError(
        `Request failed: ${err instanceof Error ? err.message : String(err)}`,
      );
    } finally {
      setLoading(false);
    }
  }, [route.method, route.path, headers, body, hasBody]);

  const statusColor = response
    ? response.status < 300
      ? "text-green-600"
      : response.status < 400
        ? "text-yellow-600"
        : "text-red-600"
    : "";

  return (
    <div>
      <div className="flex items-center justify-between mb-3">
        <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider">
          Try It
        </h4>
        <button
          onClick={sendRequest}
          disabled={loading}
          className="flex items-center gap-1.5 px-3 py-1.5 text-sm text-white bg-green-500 rounded hover:bg-green-600 disabled:opacity-50"
        >
          {loading ? (
            <Loader2 size={14} className="animate-spin" />
          ) : (
            <Play size={14} />
          )}
          Send
        </button>
      </div>

      {/* Request config */}
      <div className="space-y-3 mb-4">
        <div className="flex items-center gap-2 px-3 py-2 bg-gray-50 rounded font-mono text-sm">
          <span className="font-semibold text-blue-600">{route.method}</span>
          <span className="text-gray-700">{route.path}</span>
        </div>

        <div>
          <label className="text-xs font-medium text-gray-400 uppercase block mb-1">
            Headers
          </label>
          <div className="border border-gray-200 rounded overflow-hidden">
            <Editor
              height="60px"
              language="json"
              value={headers}
              onChange={(v) => setHeaders(v ?? "")}
              options={{
                minimap: { enabled: false },
                fontSize: 12,
                scrollBeyondLastLine: false,
                lineNumbers: "off",
                folding: false,
                wordWrap: "on",
                renderLineHighlight: "none",
                scrollbar: { vertical: "hidden" },
              }}
            />
          </div>
        </div>

        {hasBody && (
          <div>
            <label className="text-xs font-medium text-gray-400 uppercase block mb-1">
              Request Body
            </label>
            <div className="border border-gray-200 rounded overflow-hidden">
              <Editor
                height="100px"
                language="json"
                value={body}
                onChange={(v) => setBody(v ?? "")}
                options={{
                  minimap: { enabled: false },
                  fontSize: 12,
                  scrollBeyondLastLine: false,
                  lineNumbers: "on",
                  wordWrap: "on",
                }}
              />
            </div>
          </div>
        )}
      </div>

      {/* Error */}
      {error && (
        <div className="p-3 bg-red-50 border border-red-200 rounded text-sm text-red-700 mb-3">
          {error}
        </div>
      )}

      {/* Response */}
      {response && (
        <div className="space-y-2">
          <div className="flex items-center gap-3 text-sm">
            <span className={`font-mono font-semibold ${statusColor}`}>
              {response.status} {response.statusText}
            </span>
            <span className="text-gray-400">{response.duration}ms</span>
            {response.traceId && (
              <span className="text-xs text-gray-400 font-mono truncate">
                trace: {response.traceId}
              </span>
            )}
          </div>
          <div className="border border-gray-200 rounded overflow-hidden">
            <Editor
              height="200px"
              language="json"
              value={response.body}
              options={{
                minimap: { enabled: false },
                fontSize: 12,
                scrollBeyondLastLine: false,
                lineNumbers: "on",
                readOnly: true,
                wordWrap: "on",
              }}
            />
          </div>
        </div>
      )}
    </div>
  );
}
