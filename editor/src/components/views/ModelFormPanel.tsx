import { useState, useCallback, useRef, useEffect } from "react";
import { Plus, Trash2, GripVertical } from "lucide-react";
import type { ModelDefinition, ColumnDef, RelationDef } from "@/types";

const COLUMN_TYPES = [
  "uuid",
  "text",
  "varchar",
  "integer",
  "bigint",
  "boolean",
  "decimal",
  "timestamp",
  "jsonb",
  "serial",
];

const RELATION_TYPES: RelationDef["type"][] = [
  "belongsTo",
  "hasMany",
  "manyToMany",
];

interface Props {
  model: ModelDefinition;
  allTables: string[];
  onChange: (model: ModelDefinition) => void;
}

export function ModelFormPanel({ model, allTables, onChange }: Props) {
  const [newColName, setNewColName] = useState("");
  const [newRelName, setNewRelName] = useState("");
  const [newIndexCols, setNewIndexCols] = useState("");
  const [newIndexUnique, setNewIndexUnique] = useState(false);
  const [enumPopoverCol, setEnumPopoverCol] = useState<string | null>(null);
  const [enumInput, setEnumInput] = useState("");
  const enumPopoverRef = useRef<HTMLDivElement>(null);
  const [dragCol, setDragCol] = useState<string | null>(null);
  const [dragOverCol, setDragOverCol] = useState<string | null>(null);

  // Close enum popover on outside click or Escape
  useEffect(() => {
    if (!enumPopoverCol) return;
    const handleClick = (e: MouseEvent) => {
      if (
        enumPopoverRef.current &&
        !enumPopoverRef.current.contains(e.target as Node)
      ) {
        setEnumPopoverCol(null);
        setEnumInput("");
      }
    };
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setEnumPopoverCol(null);
        setEnumInput("");
      }
    };
    document.addEventListener("mousedown", handleClick);
    document.addEventListener("keydown", handleKey);
    return () => {
      document.removeEventListener("mousedown", handleClick);
      document.removeEventListener("keydown", handleKey);
    };
  }, [enumPopoverCol]);

  const updateColumn = useCallback(
    (name: string, updates: Partial<ColumnDef>) => {
      onChange({
        ...model,
        columns: {
          ...model.columns,
          [name]: { ...model.columns[name], ...updates },
        },
      });
    },
    [model, onChange],
  );

  const renameColumn = useCallback(
    (oldName: string, newName: string) => {
      if (oldName === newName || !newName || model.columns[newName]) return;
      // Rebuild columns preserving order
      const entries = Object.entries(model.columns);
      const newColumns: Record<string, ColumnDef> = {};
      for (const [key, val] of entries) {
        newColumns[key === oldName ? newName : key] = val;
      }
      // Check relations referencing old column as foreign_key
      let newRelations = model.relations;
      if (model.relations) {
        for (const [relName, rel] of Object.entries(model.relations)) {
          if (rel.foreign_key === oldName) {
            const doUpdate = confirm(
              `Column '${oldName}' is used as a foreign key in relation '${relName}'. Update the relation to use '${newName}'?`,
            );
            if (doUpdate) {
              newRelations = {
                ...(newRelations ?? {}),
                [relName]: { ...rel, foreign_key: newName },
              };
            }
          }
        }
      }
      onChange({ ...model, columns: newColumns, relations: newRelations });
    },
    [model, onChange],
  );

  const removeColumn = useCallback(
    (name: string) => {
      // eslint-disable-next-line @typescript-eslint/no-unused-vars
      const { [name]: _removed, ...rest } = model.columns;
      onChange({ ...model, columns: rest });
    },
    [model, onChange],
  );

  const addColumn = useCallback(() => {
    const name = newColName.trim();
    if (!name || model.columns[name]) return;
    const maxOrder = Math.max(
      0,
      ...Object.values(model.columns).map((c) => c.order ?? 0),
    );
    onChange({
      ...model,
      columns: {
        ...model.columns,
        [name]: { type: "text", order: maxOrder + 1 },
      },
    });
    setNewColName("");
  }, [model, newColName, onChange]);

  const updateRelation = useCallback(
    (name: string, updates: Partial<RelationDef>) => {
      onChange({
        ...model,
        relations: {
          ...model.relations,
          [name]: {
            ...(model.relations?.[name] ?? {
              type: "belongsTo",
              table: "",
              foreign_key: "",
            }),
            ...updates,
          },
        },
      });
    },
    [model, onChange],
  );

  const removeRelation = useCallback(
    (name: string) => {
      // eslint-disable-next-line @typescript-eslint/no-unused-vars
      const { [name]: _removed, ...rest } = model.relations ?? {};
      onChange({ ...model, relations: rest });
    },
    [model, onChange],
  );

  const addRelation = useCallback(() => {
    const name = newRelName.trim();
    if (!name || model.relations?.[name]) return;
    onChange({
      ...model,
      relations: {
        ...(model.relations ?? {}),
        [name]: { type: "belongsTo", table: "", foreign_key: name + "_id" },
      },
    });
    setNewRelName("");
  }, [model, newRelName, onChange]);

  const addIndex = useCallback(() => {
    const cols = newIndexCols
      .split(",")
      .map((c) => c.trim())
      .filter(Boolean);
    if (cols.length === 0) return;
    onChange({
      ...model,
      indexes: [
        ...(model.indexes ?? []),
        { columns: cols, unique: newIndexUnique || undefined },
      ],
    });
    setNewIndexCols("");
    setNewIndexUnique(false);
  }, [model, newIndexCols, newIndexUnique, onChange]);

  const removeIndex = useCallback(
    (idx: number) => {
      const indexes = [...(model.indexes ?? [])];
      indexes.splice(idx, 1);
      onChange({ ...model, indexes });
    },
    [model, onChange],
  );

  const updateIndex = useCallback(
    (idx: number, unique: boolean) => {
      const indexes = [...(model.indexes ?? [])];
      indexes[idx] = { ...indexes[idx], unique: unique || undefined };
      onChange({ ...model, indexes });
    },
    [model, onChange],
  );

  const addEnumValue = useCallback(
    (colName: string, value: string) => {
      const trimmed = value.trim();
      if (!trimmed) return;
      const col = model.columns[colName];
      const existing = col.enum ?? [];
      if (existing.includes(trimmed)) return;
      updateColumn(colName, { enum: [...existing, trimmed] });
    },
    [model, updateColumn],
  );

  const removeEnumValue = useCallback(
    (colName: string, value: string) => {
      const col = model.columns[colName];
      const updated = (col.enum ?? []).filter((v) => v !== value);
      updateColumn(colName, { enum: updated.length > 0 ? updated : undefined });
    },
    [model, updateColumn],
  );

  const reorderColumns = useCallback(
    (fromName: string, toName: string) => {
      if (fromName === toName) return;
      const fromOrder = model.columns[fromName]?.order ?? 0;
      const toOrder = model.columns[toName]?.order ?? 0;
      onChange({
        ...model,
        columns: {
          ...model.columns,
          [fromName]: { ...model.columns[fromName], order: toOrder },
          [toName]: { ...model.columns[toName], order: fromOrder },
        },
      });
    },
    [model, onChange],
  );

  const columnEntries = Object.entries(model.columns).sort(
    ([, a], [, b]) => (a.order ?? 0) - (b.order ?? 0),
  );

  return (
    <div className="p-4 space-y-6 overflow-y-auto">
      {/* Table name */}
      <div>
        <label className="text-xs font-medium text-gray-500 uppercase tracking-wider">
          Table Name
        </label>
        <input
          type="text"
          value={model.table}
          onChange={(e) => onChange({ ...model, table: e.target.value })}
          className="mt-1 w-full px-3 py-1.5 text-sm border border-gray-300 rounded font-mono focus:outline-none focus:ring-2 focus:ring-blue-400"
        />
      </div>

      {/* Columns */}
      <div>
        <h4 className="text-xs font-medium text-gray-500 uppercase tracking-wider mb-2">
          Columns ({columnEntries.length})
        </h4>
        <div className="border border-gray-200 rounded overflow-visible">
          <table className="w-full text-sm table-fixed">
            <colgroup>
              <col className="w-7" /> {/* drag handle */}
              <col style={{ width: "22%" }} /> {/* Name */}
              <col className="w-24" /> {/* Type */}
              <col className="w-8" /> {/* PK */}
              <col className="w-8" /> {/* NN */}
              <col className="w-16" /> {/* Length */}
              <col className="w-20" /> {/* P / S */}
              <col className="w-12" /> {/* Enum */}
              <col /> {/* Default — fills remaining */}
              <col className="w-8" /> {/* delete */}
            </colgroup>
            <thead>
              <tr className="bg-gray-50 text-left text-xs text-gray-500">
                <th></th>
                <th className="px-2 py-1.5">Name</th>
                <th className="px-2 py-1.5">Type</th>
                <th className="px-1 py-1.5 text-center">PK</th>
                <th className="px-1 py-1.5 text-center">NN</th>
                <th className="px-2 py-1.5">Len</th>
                <th className="px-2 py-1.5">P / S</th>
                <th className="px-1 py-1.5">Enum</th>
                <th className="px-2 py-1.5">Default</th>
                <th></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {columnEntries.map(([name, col]) => (
                <tr
                  key={name}
                  draggable
                  onDragStart={(e) => {
                    setDragCol(name);
                    e.dataTransfer.effectAllowed = "move";
                  }}
                  onDragOver={(e) => {
                    e.preventDefault();
                    e.dataTransfer.dropEffect = "move";
                    setDragOverCol(name);
                  }}
                  onDragLeave={() => {
                    if (dragOverCol === name) setDragOverCol(null);
                  }}
                  onDrop={(e) => {
                    e.preventDefault();
                    if (dragCol && dragCol !== name)
                      reorderColumns(dragCol, name);
                    setDragCol(null);
                    setDragOverCol(null);
                  }}
                  onDragEnd={() => {
                    setDragCol(null);
                    setDragOverCol(null);
                  }}
                  className={`hover:bg-gray-50 ${dragOverCol === name && dragCol !== name ? "bg-blue-50" : ""} ${dragCol === name ? "opacity-50" : ""}`}
                >
                  <td className="px-1 py-1 cursor-grab text-gray-300 hover:text-gray-500 text-center">
                    <GripVertical size={14} className="inline-block" />
                  </td>
                  <td className="px-2 py-1">
                    <input
                      type="text"
                      defaultValue={name}
                      onBlur={(e) => {
                        const newName = e.target.value.trim();
                        if (newName && newName !== name)
                          renameColumn(name, newName);
                        else e.target.value = name;
                      }}
                      onKeyDown={(e) => {
                        if (e.key === "Enter")
                          (e.target as HTMLInputElement).blur();
                      }}
                      className="text-xs border border-gray-200 rounded px-1 py-0.5 w-full font-mono"
                    />
                  </td>
                  <td className="px-2 py-1">
                    <select
                      value={col.type}
                      onChange={(e) =>
                        updateColumn(name, { type: e.target.value })
                      }
                      className="text-xs border border-gray-200 rounded px-1 py-0.5 w-full"
                    >
                      {COLUMN_TYPES.map((t) => (
                        <option key={t} value={t}>
                          {t}
                        </option>
                      ))}
                    </select>
                  </td>
                  <td className="px-1 py-1 text-center">
                    <input
                      type="checkbox"
                      checked={col.primary_key ?? false}
                      onChange={(e) =>
                        updateColumn(name, {
                          primary_key: e.target.checked || undefined,
                        })
                      }
                      className="rounded"
                    />
                  </td>
                  <td className="px-1 py-1 text-center">
                    <input
                      type="checkbox"
                      checked={col.not_null ?? false}
                      onChange={(e) =>
                        updateColumn(name, {
                          not_null: e.target.checked || undefined,
                        })
                      }
                      className="rounded"
                    />
                  </td>
                  <td className="px-2 py-1">
                    {col.type === "varchar" ? (
                      <input
                        type="number"
                        value={col.max_length ?? ""}
                        onChange={(e) =>
                          updateColumn(name, {
                            max_length: e.target.value
                              ? Number(e.target.value)
                              : undefined,
                          })
                        }
                        className="text-xs border border-gray-200 rounded px-1 py-0.5 w-full"
                        placeholder="255"
                      />
                    ) : null}
                  </td>
                  <td className="px-2 py-1">
                    {col.type === "decimal" ? (
                      <div className="flex gap-1">
                        <input
                          type="number"
                          value={col.precision ?? ""}
                          onChange={(e) =>
                            updateColumn(name, {
                              precision: e.target.value
                                ? Number(e.target.value)
                                : undefined,
                            })
                          }
                          className="text-xs border border-gray-200 rounded px-1 py-0.5 w-1/2"
                          placeholder="P"
                        />
                        <input
                          type="number"
                          value={col.scale ?? ""}
                          onChange={(e) =>
                            updateColumn(name, {
                              scale: e.target.value
                                ? Number(e.target.value)
                                : undefined,
                            })
                          }
                          className="text-xs border border-gray-200 rounded px-1 py-0.5 w-1/2"
                          placeholder="S"
                        />
                      </div>
                    ) : null}
                  </td>
                  <td className="px-1 py-1 relative">
                    {col.type === "text" || col.type === "varchar" ? (
                      <>
                        <button
                          onClick={() => {
                            setEnumPopoverCol(
                              enumPopoverCol === name ? null : name,
                            );
                            setEnumInput("");
                          }}
                          className="text-xs px-1.5 py-0.5 bg-gray-100 hover:bg-gray-200 rounded text-gray-600 font-mono"
                        >
                          {(col.enum ?? []).length}
                        </button>
                        {enumPopoverCol === name && (
                          <div
                            ref={enumPopoverRef}
                            className="absolute top-full left-0 z-10 mt-1 w-52 bg-white border border-gray-200 rounded shadow-lg p-2 space-y-2"
                          >
                            <div className="flex flex-wrap gap-1">
                              {(col.enum ?? []).map((v) => (
                                <span
                                  key={v}
                                  className="inline-flex items-center gap-1 text-xs bg-blue-50 text-blue-700 px-1.5 py-0.5 rounded"
                                >
                                  {v}
                                  <button
                                    onClick={() => removeEnumValue(name, v)}
                                    className="text-blue-400 hover:text-red-500"
                                  >
                                    ×
                                  </button>
                                </span>
                              ))}
                            </div>
                            <div className="flex gap-1">
                              <input
                                type="text"
                                value={enumInput}
                                onChange={(e) => setEnumInput(e.target.value)}
                                onKeyDown={(e) => {
                                  if (e.key === "Enter") {
                                    addEnumValue(name, enumInput);
                                    setEnumInput("");
                                  }
                                }}
                                className="text-xs border border-gray-200 rounded px-1 py-0.5 flex-1 font-mono"
                                placeholder="Add value..."
                                autoFocus
                              />
                            </div>
                          </div>
                        )}
                      </>
                    ) : null}
                  </td>
                  <td className="px-2 py-1">
                    <input
                      type="text"
                      value={col.default ?? ""}
                      onChange={(e) =>
                        updateColumn(name, {
                          default: e.target.value || undefined,
                        })
                      }
                      className="text-xs border border-gray-200 rounded px-1 py-0.5 w-full font-mono"
                      placeholder="none"
                    />
                  </td>
                  <td className="px-1 py-1 text-center">
                    <button
                      onClick={() => removeColumn(name)}
                      className="text-gray-300 hover:text-red-500"
                    >
                      <Trash2 size={14} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="flex items-center gap-2 mt-2">
          <input
            type="text"
            value={newColName}
            onChange={(e) => setNewColName(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && addColumn()}
            className="text-xs border border-gray-200 rounded px-2 py-1 font-mono flex-1"
            placeholder="column_name"
          />
          <button
            onClick={addColumn}
            disabled={!newColName.trim()}
            className="flex items-center gap-1 text-xs text-blue-600 hover:text-blue-800 disabled:opacity-30"
          >
            <Plus size={14} /> Add Column
          </button>
        </div>
      </div>

      {/* Options */}
      <div className="flex items-center gap-6">
        <label className="flex items-center gap-2 text-sm text-gray-700">
          <input
            type="checkbox"
            checked={model.timestamps ?? false}
            onChange={(e) =>
              onChange({ ...model, timestamps: e.target.checked })
            }
            className="rounded"
          />
          Timestamps (created_at, updated_at)
        </label>
        <label className="flex items-center gap-2 text-sm text-gray-700">
          <input
            type="checkbox"
            checked={model.soft_delete ?? false}
            onChange={(e) =>
              onChange({ ...model, soft_delete: e.target.checked })
            }
            className="rounded"
          />
          Soft Delete (deleted_at)
        </label>
      </div>

      {/* Relations */}
      <div>
        <h4 className="text-xs font-medium text-gray-500 uppercase tracking-wider mb-2">
          Relations ({Object.keys(model.relations ?? {}).length})
        </h4>
        <div className="space-y-2">
          {Object.entries(model.relations ?? {}).map(([name, rel]) => (
            <div
              key={name}
              className="flex items-center gap-2 p-2 bg-gray-50 rounded border border-gray-200"
            >
              <span className="text-sm font-mono text-gray-700 w-28 shrink-0">
                {name}
              </span>
              <select
                value={rel.type}
                onChange={(e) =>
                  updateRelation(name, {
                    type: e.target.value as RelationDef["type"],
                  })
                }
                className="text-xs border border-gray-200 rounded px-1 py-0.5"
              >
                {RELATION_TYPES.map((t) => (
                  <option key={t} value={t}>
                    {t}
                  </option>
                ))}
              </select>
              <span className="text-xs text-gray-400">to</span>
              <select
                value={rel.table}
                onChange={(e) =>
                  updateRelation(name, { table: e.target.value })
                }
                className="text-xs border border-gray-200 rounded px-1 py-0.5"
              >
                <option value="">Select table...</option>
                {allTables.map((t) => (
                  <option key={t} value={t}>
                    {t}
                  </option>
                ))}
              </select>
              <input
                type="text"
                value={rel.foreign_key}
                onChange={(e) =>
                  updateRelation(name, { foreign_key: e.target.value })
                }
                className="text-xs border border-gray-200 rounded px-1 py-0.5 font-mono w-28"
                placeholder="foreign_key"
              />
              {rel.type === "belongsTo" && (
                <select
                  value={rel.on_delete ?? ""}
                  onChange={(e) =>
                    updateRelation(name, {
                      on_delete: e.target.value || undefined,
                    })
                  }
                  className="text-xs border border-gray-200 rounded px-1 py-0.5"
                >
                  <option value="">ON DELETE...</option>
                  <option value="CASCADE">CASCADE</option>
                  <option value="SET NULL">SET NULL</option>
                  <option value="RESTRICT">RESTRICT</option>
                </select>
              )}
              <button
                onClick={() => removeRelation(name)}
                className="ml-auto text-gray-300 hover:text-red-500"
              >
                <Trash2 size={14} />
              </button>
            </div>
          ))}
        </div>
        <div className="flex items-center gap-2 mt-2">
          <input
            type="text"
            value={newRelName}
            onChange={(e) => setNewRelName(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && addRelation()}
            className="text-xs border border-gray-200 rounded px-2 py-1 font-mono flex-1"
            placeholder="relation_name"
          />
          <button
            onClick={addRelation}
            disabled={!newRelName.trim()}
            className="flex items-center gap-1 text-xs text-blue-600 hover:text-blue-800 disabled:opacity-30"
          >
            <Plus size={14} /> Add Relation
          </button>
        </div>
      </div>

      {/* Indexes */}
      <div>
        <h4 className="text-xs font-medium text-gray-500 uppercase tracking-wider mb-2">
          Indexes ({model.indexes?.length ?? 0})
        </h4>
        <div className="space-y-1">
          {(model.indexes ?? []).map((idx, i) => (
            <div
              key={i}
              className="flex items-center gap-2 p-2 bg-gray-50 rounded border border-gray-200"
            >
              <span className="text-xs font-mono text-gray-700">
                [{idx.columns.join(", ")}]
              </span>
              <label className="flex items-center gap-1 text-xs text-gray-600">
                <input
                  type="checkbox"
                  checked={idx.unique ?? false}
                  onChange={(e) => updateIndex(i, e.target.checked)}
                  className="rounded"
                />
                Unique
              </label>
              {idx.unique && (
                <span className="text-[10px] px-1.5 py-0.5 bg-yellow-100 text-yellow-700 rounded">
                  UNIQUE
                </span>
              )}
              <button
                onClick={() => removeIndex(i)}
                className="ml-auto text-gray-300 hover:text-red-500"
              >
                <Trash2 size={14} />
              </button>
            </div>
          ))}
        </div>
        <div className="flex items-center gap-2 mt-2">
          <input
            type="text"
            value={newIndexCols}
            onChange={(e) => setNewIndexCols(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && addIndex()}
            className="text-xs border border-gray-200 rounded px-2 py-1 font-mono flex-1"
            placeholder="col1, col2"
          />
          <label className="flex items-center gap-1 text-xs text-gray-600">
            <input
              type="checkbox"
              checked={newIndexUnique}
              onChange={(e) => setNewIndexUnique(e.target.checked)}
              className="rounded"
            />
            Unique
          </label>
          <button
            onClick={addIndex}
            disabled={!newIndexCols.trim()}
            className="flex items-center gap-1 text-xs text-blue-600 hover:text-blue-800 disabled:opacity-30"
          >
            <Plus size={14} /> Add Index
          </button>
        </div>
      </div>
    </div>
  );
}
