import type { SchemaInfo } from "@/types";
import { groupByFolder } from "@/utils/schemaUtils";

interface SchemaSelectProps {
  schemas: SchemaInfo[];
  value: string;
  onChange: (value: string) => void;
  className?: string;
  placeholder?: string;
}

export function SchemaSelect({
  schemas,
  value,
  onChange,
  className,
  placeholder = "No schema",
}: SchemaSelectProps) {
  const groups = groupByFolder(schemas);

  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className={className}
    >
      <option value="">{placeholder}</option>
      {[...groups.entries()].map(([folder, items]) => {
        if (!folder) {
          return items.map((s) => (
            <option key={s.path} value={s.path}>
              {s.path}
            </option>
          ));
        }
        return (
          <optgroup key={folder} label={folder}>
            {items.map((s) => (
              <option key={s.path} value={s.path}>
                {s.path}
              </option>
            ))}
          </optgroup>
        );
      })}
    </select>
  );
}
