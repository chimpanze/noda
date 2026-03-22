import { useState } from "react";
import type { StreamStatus } from "../types";

interface Props {
  status: StreamStatus;
  onJoin: (name: string) => void;
  joining: boolean;
}

export default function ViewerJoin({ status, onJoin, joining }: Props) {
  const [name, setName] = useState("");

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (name.trim()) onJoin(name.trim());
  };

  return (
    <div className="flex flex-col items-center justify-center gap-6 py-16 text-center">
      <div className="flex items-center gap-2 text-sm font-medium text-green-400">
        <span className="inline-block h-2 w-2 animate-pulse rounded-full bg-green-400" />
        LIVE
      </div>
      <h2 className="text-3xl font-bold text-white">{status.title}</h2>
      {status.description && (
        <p className="max-w-lg text-gray-400">{status.description}</p>
      )}
      <form onSubmit={handleSubmit} className="mt-4 flex gap-3">
        <input
          type="text"
          placeholder="Your name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          required
          className="rounded-lg border border-gray-600 bg-gray-800 px-4 py-2 text-white placeholder-gray-500 focus:border-blue-500 focus:outline-none"
        />
        <button
          type="submit"
          disabled={joining || !name.trim()}
          className="rounded-lg bg-blue-600 px-6 py-2 font-medium text-white transition hover:bg-blue-700 disabled:opacity-50"
        >
          {joining ? "Joining..." : "Watch Stream"}
        </button>
      </form>
    </div>
  );
}
