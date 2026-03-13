import { useState, useCallback } from "react";
import { Trash2, Plus, ChevronDown, ChevronRight, Pencil } from "lucide-react";

const PROPERTY_TYPES = [
  "string",
  "number",
  "integer",
  "boolean",
  "object",
  "array",
];

interface SchemaDefinition {
  type?: string;
  properties?: Record<string, PropertyDef>;
  required?: string[];
  [key: string]: unknown;
}

interface PropertyDef {
  type?: string;
  enum?: string[];
  [key: string]: unknown;
}

interface SchemaPropertyEditorProps {
  /** Full file content: top-level keys are schema names, values are schema objects */
  content: Record<string, SchemaDefinition>;
  onChange: (content: Record<string, SchemaDefinition>) => void;
}

export function SchemaPropertyEditor({
  content,
  onChange,
}: SchemaPropertyEditorProps) {
  const schemaNames = Object.keys(content);

  const updateSchema = useCallback(
    (name: string, schema: SchemaDefinition) => {
      onChange({ ...content, [name]: schema });
    },
    [content, onChange],
  );

  const removeSchema = useCallback(
    (name: string) => {
      const next = { ...content };
      delete next[name];
      onChange(next);
    },
    [content, onChange],
  );

  const renameSchema = useCallback(
    (oldName: string, newName: string) => {
      if (!newName || newName === oldName || newName in content) return;
      const entries = Object.entries(content);
      const result: Record<string, SchemaDefinition> = {};
      for (const [k, v] of entries) {
        result[k === oldName ? newName : k] = v;
      }
      onChange(result);
    },
    [content, onChange],
  );

  const addSchema = useCallback(() => {
    let name = "NewSchema";
    let i = 1;
    while (name in content) name = `NewSchema${i++}`;
    onChange({ ...content, [name]: { type: "object", properties: {} } });
  }, [content, onChange]);

  return (
    <div className="p-4 space-y-4">
      {schemaNames.map((name) => (
        <SchemaSection
          key={name}
          name={name}
          schema={content[name]}
          onChange={(s) => updateSchema(name, s)}
          onRemove={() => removeSchema(name)}
          onRename={(n) => renameSchema(name, n)}
        />
      ))}
      <button
        type="button"
        onClick={addSchema}
        className="flex items-center gap-1 text-xs text-blue-500 hover:text-blue-700"
      >
        <Plus size={12} />
        Add schema to file
      </button>
    </div>
  );
}

function SchemaSection({
  name,
  schema,
  onChange,
  onRemove,
  onRename,
}: {
  name: string;
  schema: SchemaDefinition;
  onChange: (schema: SchemaDefinition) => void;
  onRemove: () => void;
  onRename: (name: string) => void;
}) {
  const [collapsed, setCollapsed] = useState(false);
  const [renaming, setRenaming] = useState(false);
  const [renameDraft, setRenameDraft] = useState(name);

  const properties = schema.properties ?? {};
  const required = schema.required ?? [];
  const propNames = Object.keys(properties);

  const updateProperty = (propName: string, prop: PropertyDef) => {
    onChange({ ...schema, properties: { ...properties, [propName]: prop } });
  };

  const removeProperty = (propName: string) => {
    const next = { ...properties };
    delete next[propName];
    onChange({
      ...schema,
      properties: next,
      required: required.filter((r) => r !== propName),
    });
  };

  const renameProperty = (oldName: string, newName: string) => {
    if (!newName || newName === oldName || newName in properties) return;
    const entries = Object.entries(properties);
    const nextProps: Record<string, PropertyDef> = {};
    for (const [k, v] of entries) {
      nextProps[k === oldName ? newName : k] = v;
    }
    onChange({
      ...schema,
      properties: nextProps,
      required: required.map((r) => (r === oldName ? newName : r)),
    });
  };

  const toggleRequired = (propName: string) => {
    if (required.includes(propName)) {
      onChange({ ...schema, required: required.filter((r) => r !== propName) });
    } else {
      onChange({ ...schema, required: [...required, propName] });
    }
  };

  const addProperty = () => {
    let propName = "newField";
    let i = 1;
    while (propName in properties) propName = `newField${i++}`;
    onChange({
      ...schema,
      properties: { ...properties, [propName]: { type: "string" } },
    });
  };

  const commitRename = () => {
    setRenaming(false);
    if (renameDraft.trim() && renameDraft !== name) {
      onRename(renameDraft.trim());
    }
  };

  return (
    <div className="border border-gray-200 rounded">
      <div className="flex items-center gap-2 px-3 py-2 bg-gray-50 border-b border-gray-200">
        <button
          type="button"
          onClick={() => setCollapsed(!collapsed)}
          className="text-gray-400 hover:text-gray-600"
        >
          {collapsed ? <ChevronRight size={14} /> : <ChevronDown size={14} />}
        </button>
        {renaming ? (
          <input
            type="text"
            value={renameDraft}
            onChange={(e) => setRenameDraft(e.target.value)}
            onBlur={commitRename}
            onKeyDown={(e) => {
              if (e.key === "Enter") commitRename();
              if (e.key === "Escape") {
                setRenaming(false);
                setRenameDraft(name);
              }
            }}
            className="text-sm font-medium font-mono bg-white border border-blue-300 rounded px-1 py-0.5 focus:outline-none focus:ring-1 focus:ring-blue-400"
            autoFocus
          />
        ) : (
          <span className="text-sm font-medium font-mono text-gray-800">
            {name}
          </span>
        )}
        <div className="flex-1" />
        <button
          type="button"
          onClick={() => {
            setRenameDraft(name);
            setRenaming(true);
          }}
          className="text-xs text-gray-400 hover:text-gray-600"
          title="Rename"
        >
          <Pencil size={12} />
        </button>
        <button
          type="button"
          onClick={() => {
            if (confirm(`Remove schema "${name}"?`)) onRemove();
          }}
          className="text-xs text-red-400 hover:text-red-600"
          title="Remove"
        >
          <Trash2 size={12} />
        </button>
      </div>

      {!collapsed && (
        <div className="p-3">
          {propNames.length > 0 && (
            <table className="w-full text-sm">
              <thead>
                <tr className="text-[10px] text-gray-400 uppercase tracking-wider">
                  <th className="text-left pb-1 font-medium">Name</th>
                  <th className="text-left pb-1 font-medium w-28">Type</th>
                  <th className="text-center pb-1 font-medium w-16">Req</th>
                  <th className="text-left pb-1 font-medium">Enum</th>
                  <th className="pb-1 w-8" />
                </tr>
              </thead>
              <tbody>
                {propNames.map((propName) => (
                  <PropertyRow
                    key={propName}
                    name={propName}
                    prop={properties[propName]}
                    isRequired={required.includes(propName)}
                    onUpdate={(p) => updateProperty(propName, p)}
                    onRemove={() => removeProperty(propName)}
                    onRename={(n) => renameProperty(propName, n)}
                    onToggleRequired={() => toggleRequired(propName)}
                  />
                ))}
              </tbody>
            </table>
          )}
          {propNames.length === 0 && (
            <p className="text-xs text-gray-400 mb-2">No properties defined.</p>
          )}
          <button
            type="button"
            onClick={addProperty}
            className="flex items-center gap-1 mt-2 text-xs text-blue-500 hover:text-blue-700"
          >
            <Plus size={12} />
            Add property
          </button>
        </div>
      )}
    </div>
  );
}

function PropertyRow({
  name,
  prop,
  isRequired,
  onUpdate,
  onRemove,
  onRename,
  onToggleRequired,
}: {
  name: string;
  prop: PropertyDef;
  isRequired: boolean;
  onUpdate: (prop: PropertyDef) => void;
  onRemove: () => void;
  onRename: (name: string) => void;
  onToggleRequired: () => void;
}) {
  const [editingName, setEditingName] = useState(false);
  const [nameDraft, setNameDraft] = useState(name);

  const commitName = () => {
    setEditingName(false);
    if (nameDraft.trim() && nameDraft !== name) {
      onRename(nameDraft.trim());
    }
  };

  const enumStr =
    prop.type === "string" && prop.enum ? prop.enum.join(", ") : "";

  return (
    <tr className="border-t border-gray-100">
      <td className="py-1 pr-2">
        {editingName ? (
          <input
            type="text"
            value={nameDraft}
            onChange={(e) => setNameDraft(e.target.value)}
            onBlur={commitName}
            onKeyDown={(e) => {
              if (e.key === "Enter") commitName();
              if (e.key === "Escape") {
                setEditingName(false);
                setNameDraft(name);
              }
            }}
            className="w-full text-xs font-mono bg-white border border-blue-300 rounded px-1 py-0.5 focus:outline-none focus:ring-1 focus:ring-blue-400"
            autoFocus
          />
        ) : (
          <button
            type="button"
            onClick={() => {
              setNameDraft(name);
              setEditingName(true);
            }}
            className="text-xs font-mono text-gray-800 hover:text-blue-600 text-left"
          >
            {name}
          </button>
        )}
      </td>
      <td className="py-1 pr-2">
        <select
          value={prop.type ?? "string"}
          onChange={(e) => {
            const newType = e.target.value;
            const next: PropertyDef = { ...prop, type: newType };
            // Clear enum if switching away from string
            if (newType !== "string" && next.enum) delete next.enum;
            onUpdate(next);
          }}
          className="w-full text-xs border border-gray-200 rounded px-1 py-0.5 bg-white"
        >
          {PROPERTY_TYPES.map((t) => (
            <option key={t} value={t}>
              {t}
            </option>
          ))}
        </select>
      </td>
      <td className="py-1 text-center">
        <input
          type="checkbox"
          checked={isRequired}
          onChange={onToggleRequired}
          className="rounded border-gray-300"
        />
      </td>
      <td className="py-1 pr-2">
        {prop.type === "string" && (
          <input
            type="text"
            value={enumStr}
            onChange={(e) => {
              const val = e.target.value;
              if (!val.trim()) {
                // eslint-disable-next-line @typescript-eslint/no-unused-vars
                const { enum: _enum, ...rest } = prop;
                onUpdate(rest);
              } else {
                onUpdate({
                  ...prop,
                  enum: val
                    .split(",")
                    .map((s) => s.trim())
                    .filter(Boolean),
                });
              }
            }}
            className="w-full text-xs border border-gray-200 rounded px-1 py-0.5 font-mono"
            placeholder="val1, val2, ..."
          />
        )}
      </td>
      <td className="py-1 text-right">
        <button
          type="button"
          onClick={onRemove}
          className="text-red-400 hover:text-red-600"
        >
          <Trash2 size={12} />
        </button>
      </td>
    </tr>
  );
}
