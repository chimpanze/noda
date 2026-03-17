import { ExpressionAutocomplete } from "@/components/widgets/ExpressionAutocomplete";

export function KeyValueEditor({
  entries,
  onChange,
  workflow,
}: {
  entries: Record<string, string>;
  onChange: (entries: Record<string, string>) => void;
  workflow?: string;
}) {
  const pairs = Object.entries(entries);

  return (
    <>
      <div className="space-y-1">
        {pairs.map(([key, val]) => (
          <div key={key} className="flex items-center gap-1">
            <input
              type="text"
              value={key}
              onChange={(e) => {
                const next: Record<string, string> = {};
                for (const [k, v] of pairs)
                  next[k === key ? e.target.value : k] = v;
                onChange(next);
              }}
              className="shrink-0 input-field !w-1/3 font-mono"
              placeholder="key"
            />
            <span className="text-gray-400 text-xs">:</span>
            {workflow ? (
              <div className="flex-1 min-w-0">
                <ExpressionAutocomplete
                  value={val}
                  onChange={(v) => onChange({ ...entries, [key]: v })}
                  workflow={workflow}
                  className="input-field !w-auto font-mono"
                  placeholder="value"
                />
              </div>
            ) : (
              <input
                type="text"
                value={val}
                onChange={(e) =>
                  onChange({ ...entries, [key]: e.target.value })
                }
                className="flex-1 min-w-0 input-field !w-auto font-mono"
                placeholder="value"
              />
            )}
            <button
              type="button"
              onClick={() => {
                const next = { ...entries };
                delete next[key];
                onChange(next);
              }}
              className="px-1 text-red-400 hover:text-red-600 text-sm"
            >
              &times;
            </button>
          </div>
        ))}
      </div>
      <button
        type="button"
        onClick={() => {
          let k = "key";
          let i = 1;
          while (k in entries) k = `key${i++}`;
          onChange({ ...entries, [k]: "" });
        }}
        className="mt-1 text-xs text-blue-500 hover:text-blue-700"
      >
        + Add field
      </button>
    </>
  );
}
