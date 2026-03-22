export default function OfflineState() {
  return (
    <div className="flex flex-col items-center justify-center gap-4 py-20 text-center">
      <div className="text-6xl">🏔️</div>
      <h2 className="text-2xl font-bold text-gray-200">Stream is Offline</h2>
      <p className="max-w-md text-gray-400">
        No adventure is live right now. Check back later or wait here — the page
        will update automatically when the stream starts.
      </p>
      <div className="mt-4 flex items-center gap-2 text-sm text-gray-500">
        <span className="inline-block h-2 w-2 rounded-full bg-gray-500" />
        Checking every few seconds...
      </div>
    </div>
  );
}
