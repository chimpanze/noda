import { useState, useEffect, useCallback } from "react";
import { LiveKitRoom, RoomAudioRenderer } from "@livekit/components-react";
import { getStreamToken, getAdminStreamStatus } from "../api";
import ChatPanel from "./ChatPanel";

export default function ChatMonitor() {
  const [token, setToken] = useState<string | null>(null);
  const [serverUrl, setServerUrl] = useState<string | null>(null);
  const [isLive, setIsLive] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fullscreen, setFullscreen] = useState(false);

  const checkStatus = useCallback(async () => {
    try {
      const s = await getAdminStreamStatus();
      const live = !!s.stream;
      setIsLive(live);
      if (live && !token) {
        const t = await getStreamToken("Admin (monitor)");
        setToken(t.token);
        setServerUrl(t.url);
      }
      if (!live) {
        setToken(null);
        setServerUrl(null);
      }
    } catch {
      // ignore
    }
  }, [token]);

  useEffect(() => {
    checkStatus();
    const id = setInterval(checkStatus, 10000);
    return () => clearInterval(id);
  }, [checkStatus]);

  const chatContent = !isLive ? (
    <p className="p-4 text-sm text-gray-500">
      Chat is available when the stream is live.
    </p>
  ) : error ? (
    <p className="p-4 text-sm text-red-400">{error}</p>
  ) : token && serverUrl ? (
    <LiveKitRoom
      token={token}
      serverUrl={serverUrl}
      connect={true}
      video={false}
      audio={false}
      onError={(e) => setError(e.message)}
      className="h-full"
    >
      <ChatPanel readOnly />
      <RoomAudioRenderer />
    </LiveKitRoom>
  ) : (
    <p className="p-4 text-sm text-gray-500">Connecting to chat...</p>
  );

  if (fullscreen) {
    return (
      <div className="fixed inset-0 z-50 flex flex-col bg-gray-950">
        <div className="flex items-center justify-between border-b border-gray-800 px-4 py-3">
          <h3 className="text-lg font-semibold text-white">Live Chat</h3>
          <button
            onClick={() => setFullscreen(false)}
            className="rounded-lg bg-gray-800 px-3 py-1.5 text-sm text-gray-300 transition hover:bg-gray-700"
          >
            Exit Fullscreen
          </button>
        </div>
        <div className="flex-1 overflow-hidden">{chatContent}</div>
      </div>
    );
  }

  return (
    <div className="rounded-xl border border-gray-800 bg-gray-900 p-4 sm:p-6">
      <div className="mb-4 flex items-center justify-between">
        <h3 className="text-lg font-semibold text-white">Live Chat</h3>
        {isLive && token && (
          <button
            onClick={() => setFullscreen(true)}
            className="text-sm text-gray-400 transition hover:text-white"
          >
            Fullscreen
          </button>
        )}
      </div>
      <div className="h-80">{chatContent}</div>
    </div>
  );
}
