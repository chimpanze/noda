import { useState, useRef, useEffect } from "react";
import { useChat } from "@livekit/components-react";

interface Props {
  readOnly?: boolean;
}

export default function ChatPanel({ readOnly }: Props) {
  const { chatMessages, send, isSending } = useChat();
  const [message, setMessage] = useState("");
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [chatMessages.length]);

  const handleSend = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!message.trim() || isSending) return;
    await send(message.trim());
    setMessage("");
  };

  return (
    <div className="flex h-full flex-col">
      <div className="flex-1 overflow-y-auto p-3">
        {chatMessages.length === 0 ? (
          <p className="text-center text-sm text-gray-500">
            {readOnly ? "No messages yet." : "No messages yet. Say hi!"}
          </p>
        ) : (
          <div className="space-y-2">
            {chatMessages.map((msg, i) => (
              <div key={i} className="text-sm">
                <span className="font-medium text-blue-400">
                  {msg.from?.name ?? msg.from?.identity ?? "Unknown"}
                </span>
                <span className="ml-1 text-xs text-gray-500">
                  {new Date(msg.timestamp).toLocaleTimeString([], {
                    hour: "2-digit",
                    minute: "2-digit",
                  })}
                </span>
                <p className="text-gray-200">{msg.message}</p>
              </div>
            ))}
            <div ref={bottomRef} />
          </div>
        )}
      </div>
      {!readOnly && (
        <form onSubmit={handleSend} className="border-t border-gray-700 p-3">
          <div className="flex gap-2">
            <input
              type="text"
              value={message}
              onChange={(e) => setMessage(e.target.value)}
              placeholder="Type a message..."
              className="flex-1 rounded-lg border border-gray-600 bg-gray-800 px-3 py-1.5 text-sm text-white placeholder-gray-500 focus:border-blue-500 focus:outline-none"
            />
            <button
              type="submit"
              disabled={isSending || !message.trim()}
              className="rounded-lg bg-blue-600 px-3 py-1.5 text-sm font-medium text-white transition hover:bg-blue-700 disabled:opacity-50"
            >
              Send
            </button>
          </div>
        </form>
      )}
    </div>
  );
}
