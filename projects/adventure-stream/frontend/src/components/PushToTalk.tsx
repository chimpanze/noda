import { useState, useCallback } from "react";
import { useLocalParticipant } from "@livekit/components-react";

export default function PushToTalk() {
  const { localParticipant } = useLocalParticipant();
  const [active, setActive] = useState(false);

  const startTalking = useCallback(async () => {
    try {
      await localParticipant.setMicrophoneEnabled(true);
      setActive(true);
    } catch {
      // permission denied or not available
    }
  }, [localParticipant]);

  const stopTalking = useCallback(async () => {
    try {
      await localParticipant.setMicrophoneEnabled(false);
      setActive(false);
    } catch {
      // ignore
    }
  }, [localParticipant]);

  return (
    <button
      onPointerDown={startTalking}
      onPointerUp={stopTalking}
      onPointerLeave={stopTalking}
      onContextMenu={(e) => e.preventDefault()}
      className={`rounded-full p-3 shadow-lg transition select-none ${
        active
          ? "bg-red-500 text-white scale-110"
          : "bg-gray-700 text-gray-300 hover:bg-gray-600"
      }`}
      title="Hold to speak"
    >
      <svg className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth={2}
          d="M19 11a7 7 0 01-7 7m0 0a7 7 0 01-7-7m7 7v4m0 0H8m4 0h4M12 15a3 3 0 003-3V5a3 3 0 00-6 0v7a3 3 0 003 3z"
        />
      </svg>
    </button>
  );
}
