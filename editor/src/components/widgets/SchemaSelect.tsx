import type { SchemaInfo } from "@/types";

interface SchemaSelectProps {
  schemas: SchemaInfo[];
  value: string;
  onChange: (value: string) => void;
  className?: string;
  placeholder?: string;
}

/** Flatten schema files into individual $ref options.
 *  e.g. file "schemas/greeting.json" with keys {"greeting": {...}}
 *  becomes ref "schemas/greeting". */
function buildRefOptions(
  schemas: SchemaInfo[],
): { ref: string; label: string }[] {
  const options: { ref: string; label: string }[] = [];
  for (const s of schemas) {
    // Derive the directory portion: "schemas/greeting.json" → "schemas"
    // or "schemas/auth/tokens.json" → "schemas/auth"
    const dir = s.path.replace(/\/[^/]+$/, "");
    for (const key of Object.keys(s.schema)) {
      const ref = `${dir}/${key}`;
      options.push({ ref, label: ref });
    }
  }
  options.sort((a, b) => a.label.localeCompare(b.label));
  return options;
}

export function SchemaSelect({
  schemas,
  value,
  onChange,
  className,
  placeholder = "No schema",
}: SchemaSelectProps) {
  const options = buildRefOptions(schemas);

  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className={className}
    >
      <option value="">{placeholder}</option>
      {options.map((o) => (
        <option key={o.ref} value={o.ref}>
          {o.label}
        </option>
      ))}
    </select>
  );
}
