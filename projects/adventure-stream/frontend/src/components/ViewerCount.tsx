import { useRemoteParticipants } from "@livekit/components-react";

export default function ViewerCount() {
  const participants = useRemoteParticipants();
  const count = participants.length;

  return (
    <div className="flex items-center gap-2 text-sm text-gray-400">
      <svg className="h-4 w-4" fill="currentColor" viewBox="0 0 20 20">
        <path d="M10 9a3 3 0 100-6 3 3 0 000 6zm-7 9a7 7 0 1114 0H3z" />
      </svg>
      <span>
        {count} viewer{count !== 1 ? "s" : ""}
      </span>
    </div>
  );
}
