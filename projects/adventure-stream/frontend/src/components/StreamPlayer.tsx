import { useState } from "react";
import {
  LiveKitRoom,
  RoomAudioRenderer,
  VideoTrack,
  useTracks,
} from "@livekit/components-react";
import { Track } from "livekit-client";
import ChatPanel from "./ChatPanel";
import ViewerCount from "./ViewerCount";
import PushToTalk from "./PushToTalk";

interface Props {
  token: string;
  serverUrl: string;
  onDisconnected?: () => void;
}

function VideoDisplay() {
  const tracks = useTracks([Track.Source.Camera, Track.Source.ScreenShare], {
    onlySubscribed: true,
  });

  if (tracks.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-gray-400">
        <div className="text-center">
          <div className="mb-2 text-4xl">📡</div>
          <p>Waiting for stream video...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="relative h-full w-full bg-black">
      <VideoTrack
        trackRef={tracks[0]!}
        className="h-full w-full object-contain"
      />
    </div>
  );
}

export default function StreamPlayer({
  token,
  serverUrl,
  onDisconnected,
}: Props) {
  const [chatOpen, setChatOpen] = useState(false);

  return (
    <LiveKitRoom
      token={token}
      serverUrl={serverUrl}
      connect={true}
      video={false}
      audio={false}
      onDisconnected={onDisconnected}
      className="h-full w-full"
    >
      <div className="relative flex h-full">
        <div className="flex-1">
          <VideoDisplay />
        </div>

        {chatOpen && (
          <div className="flex w-80 shrink-0 flex-col border-l border-gray-700 bg-gray-900">
            <div className="flex items-center justify-between border-b border-gray-700 px-3 py-2">
              <span className="text-sm font-medium text-white">Chat</span>
              <button
                onClick={() => setChatOpen(false)}
                className="text-gray-400 transition hover:text-white"
              >
                <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
            <ChatPanel />
          </div>
        )}

        <div className="absolute left-4 top-4">
          <ViewerCount />
        </div>

        <div className="absolute right-4 bottom-4 flex items-center gap-3">
          <PushToTalk />
          {!chatOpen && (
            <button
              onClick={() => setChatOpen(true)}
              className="rounded-full bg-blue-600 p-3 text-white shadow-lg transition hover:bg-blue-700"
              title="Open chat"
            >
              <svg className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
              </svg>
            </button>
          )}
        </div>

        <RoomAudioRenderer />
      </div>
    </LiveKitRoom>
  );
}
