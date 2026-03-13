import { useState, useEffect } from "react";
import * as api from "@/api/client";

interface Props {
  value: string;
  onChange: (value: string) => void;
}

export function TableSelectWidget({ value, onChange }: Props) {
  const [tables, setTables] = useState<string[]>([]);

  useEffect(() => {
    api.listModels().then((models) => {
      setTables(models.map((m) => m.model.table).filter(Boolean).sort());
    });
  }, []);

  return (
    <select
      value={value ?? ""}
      onChange={(e) => onChange(e.target.value)}
      className="w-full px-2 py-1 text-sm border border-gray-300 rounded font-mono focus:outline-none focus:ring-2 focus:ring-blue-400"
    >
      <option value="">Select table...</option>
      {tables.map((t) => (
        <option key={t} value={t}>{t}</option>
      ))}
    </select>
  );
}
