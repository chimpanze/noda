import { useState } from "react";
import { hasToken, clearToken } from "../api";
import AdminLogin from "../components/AdminLogin";
import IngressPanel from "../components/IngressPanel";
import StreamControl from "../components/StreamControl";
import ParticipantList from "../components/ParticipantList";
import StreamStats from "../components/StreamStats";
import ChatMonitor from "../components/ChatMonitor";

export default function AdminPage() {
  const [loggedIn, setLoggedIn] = useState(hasToken());

  if (!loggedIn) {
    return <AdminLogin onLogin={() => setLoggedIn(true)} />;
  }

  return (
    <div className="min-h-screen bg-gray-950 text-white">
      <header className="flex items-center justify-between border-b border-gray-800 px-4 py-3 sm:px-6 sm:py-4">
        <h1 className="text-lg font-bold sm:text-xl">Admin</h1>
        <div className="flex items-center gap-3 sm:gap-4">
          <a href="/" className="text-sm text-gray-400 hover:text-white">
            Viewer
          </a>
          <button
            onClick={() => {
              clearToken();
              setLoggedIn(false);
            }}
            className="text-sm text-red-400 hover:text-red-300"
          >
            Logout
          </button>
        </div>
      </header>
      <main className="mx-auto max-w-4xl space-y-4 p-4 sm:space-y-6 sm:p-6">
        <StreamStats />
        <StreamControl />
        <IngressPanel />
        <ParticipantList />
        <ChatMonitor />
      </main>
    </div>
  );
}
