import type { SchemaInfo } from "@/types";

function groupByFolder(schemas: SchemaInfo[]): Map<string, SchemaInfo[]> {
  const groups = new Map<string, SchemaInfo[]>();
  for (const s of schemas) {
    const withoutPrefix = s.path.replace(/^schemas\//, "");
    const lastSlash = withoutPrefix.lastIndexOf("/");
    const folder = lastSlash >= 0 ? withoutPrefix.substring(0, lastSlash) : "";
    const group = groups.get(folder) ?? [];
    group.push(s);
    groups.set(folder, group);
  }
  return groups;
}

export { groupByFolder };

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
