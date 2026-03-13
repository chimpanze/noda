import { useState, useEffect, useMemo } from "react";
import * as api from "@/api/client";
import type { ModelInfo } from "@/types";

interface Props {
  value: string[];
  table: string;
  onChange: (value: string[]) => void;
}

export function ColumnSelectWidget({ value, table, onChange }: Props) {
  const [models, setModels] = useState<ModelInfo[]>([]);

  useEffect(() => {
    api.listModels().then(setModels);
  }, []);

  const columns = useMemo(() => {
    const model = models.find((m) => m.model.table === table);
    if (!model) return [];
    return Object.keys(model.model.columns).sort();
  }, [models, table]);

  const toggleColumn = (col: string) => {
    if (value.includes(col)) {
      onChange(value.filter((c) => c !== col));
    } else {
      onChange([...value, col]);
    }
  };

  if (!table) {
    return <div className="text-xs text-gray-400">Select a table first</div>;
  }

  if (columns.length === 0) {
    return <div className="text-xs text-gray-400">No model found for "{table}"</div>;
  }

  return (
    <div className="space-y-1">
      {columns.map((col) => (
        <label key={col} className="flex items-center gap-2 text-sm text-gray-700">
          <input
            type="checkbox"
            checked={value.includes(col)}
            onChange={() => toggleColumn(col)}
            className="rounded"
          />
          <span className="font-mono text-xs">{col}</span>
        </label>
      ))}
    </div>
  );
}
