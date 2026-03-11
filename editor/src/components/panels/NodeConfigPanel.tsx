import { useEffect, useState, useCallback } from "react";
import Form, { type IChangeEvent } from "@rjsf/core";
import validator from "@rjsf/validator-ajv8";
import type { RJSFSchema, UiSchema } from "@rjsf/utils";
import { useEditorStore } from "@/stores/editor";
import * as api from "@/api/client";
import { updateExpressionContext } from "@/utils/expressionLanguage";
import { ExpressionWidget } from "./ExpressionWidget";
import { ServiceSlotWidget } from "./ServiceSlotWidget";
import { EnumSelectWidget } from "@/components/widgets/EnumSelectWidget";
import { BooleanToggleWidget } from "@/components/widgets/BooleanToggleWidget";
import { NumberWidget } from "@/components/widgets/NumberWidget";
import { StringArrayField } from "@/components/widgets/StringArrayField";
import { KeyValueMapField } from "@/components/widgets/KeyValueMapField";
import { FlexibleValueField } from "@/components/widgets/FlexibleValueField";
import { StyledObjectFieldTemplate } from "@/components/widgets/StyledObjectFieldTemplate";

// Custom widget registry for RJSF
const widgets = {
  expression: ExpressionWidget,
  enumSelect: EnumSelectWidget,
  booleanToggle: BooleanToggleWidget,
  number: NumberWidget,
};

// Custom field registry for RJSF
const fields = {
  stringArray: StringArrayField,
  keyValueMap: KeyValueMapField,
  flexibleValue: FlexibleValueField,
};

export function NodeConfigPanel() {
  const selectedNodeId = useEditorStore((s) => s.selectedNodeId);
  const activeWorkflow = useEditorStore((s) => s.activeWorkflow);
  const updateNodeConfig = useEditorStore((s) => s.updateNodeConfig);
  const updateNodeServices = useEditorStore((s) => s.updateNodeServices);
  const renameNode = useEditorStore((s) => s.renameNode);
  const updateNodeAlias = useEditorStore((s) => s.updateNodeAlias);
  const nodeTypes = useEditorStore((s) => s.nodeTypes);
  const saveStatus = useEditorStore((s) => s.saveStatus);

  const [schema, setSchema] = useState<RJSFSchema | null>(null);
  const [loading, setLoading] = useState(false);

  const node = activeWorkflow?.nodes.find((n) => n.id === selectedNodeId);
  const descriptor = nodeTypes.find((nt) => nt.type === node?.type);

  // Fetch schema when node type changes
  useEffect(() => {
    if (!node?.type) {
      setSchema(null);
      return;
    }
    let cancelled = false;
    setLoading(true);
    api.getNodeSchema(node.type).then((s) => {
      if (!cancelled) {
        setSchema(Object.keys(s).length > 0 ? (s as RJSFSchema) : null);
        setLoading(false);
      }
    }).catch(() => {
      if (!cancelled) {
        setSchema(null);
        setLoading(false);
      }
    });
    return () => { cancelled = true; };
  }, [node?.type]);

  // Update expression autocomplete context when node selection changes
  const activeWorkflowPath = useEditorStore((s) => s.activeWorkflowPath);
  useEffect(() => {
    if (!activeWorkflowPath || !selectedNodeId) return;
    const wfName = activeWorkflowPath.replace(/^workflows\//, "").replace(/\.json$/, "");
    updateExpressionContext(wfName, selectedNodeId);
  }, [activeWorkflowPath, selectedNodeId]);

  const onConfigChange = useCallback(
    (data: IChangeEvent<Record<string, unknown>>) => {
      if (selectedNodeId && data.formData) {
        updateNodeConfig(selectedNodeId, data.formData);
      }
    },
    [selectedNodeId, updateNodeConfig]
  );

  const onServicesChange = useCallback(
    (slot: string, value: string) => {
      if (!selectedNodeId || !node) return;
      const services = { ...(node.services ?? {}), [slot]: value };
      if (!value) delete services[slot];
      updateNodeServices(selectedNodeId, services);
    },
    [selectedNodeId, node, updateNodeServices]
  );

  if (!selectedNodeId || !node) {
    return (
      <div className="p-4 text-sm text-gray-400">
        Select a node to configure it.
      </div>
    );
  }

  // Build uiSchema — assign widgets/fields based on schema type
  const uiSchema: UiSchema = {};
  if (schema?.properties) {
    for (const [key, prop] of Object.entries(schema.properties)) {
      const p = prop as Record<string, unknown>;
      if (p.enum) {
        uiSchema[key] = { "ui:widget": "enumSelect" };
      } else if (p.type === "string") {
        uiSchema[key] = { "ui:widget": "expression" };
      } else if (p.type === "boolean") {
        uiSchema[key] = { "ui:widget": "booleanToggle" };
      } else if (p.type === "integer" || p.type === "number") {
        uiSchema[key] = { "ui:widget": "number" };
      } else if (p.type === "array" && (!p.items || (p.items as Record<string, unknown>)?.type === "string")) {
        uiSchema[key] = { "ui:field": "stringArray" };
      } else if (p.type === "object" && !p.properties) {
        uiSchema[key] = { "ui:field": "keyValueMap" };
      } else if (!p.type && !p.enum && !p.properties) {
        uiSchema[key] = { "ui:field": "flexibleValue" };
      }
    }
  }

  return (
    <div className="flex flex-col h-full overflow-hidden">
      {/* Header */}
      <div className="px-4 py-3 border-b border-gray-200 shrink-0">
        <div className="flex items-center justify-between mb-2">
          <div className="text-xs font-mono text-gray-400">{node.type}</div>
          <SaveIndicator status={saveStatus} />
        </div>
        <div className="space-y-1.5">
          <div>
            <label className="text-xs text-gray-500">ID</label>
            <input
              type="text"
              value={node.id}
              onChange={(e) => renameNode(node.id, e.target.value)}
              className="w-full px-2 py-1 text-sm border border-gray-300 rounded font-mono focus:outline-none focus:ring-2 focus:ring-blue-400 focus:border-transparent"
            />
          </div>
          <div>
            <label className="text-xs text-gray-500">Alias</label>
            <input
              type="text"
              value={node.as ?? ""}
              onChange={(e) => updateNodeAlias(node.id, e.target.value || undefined)}
              className="w-full px-2 py-1 text-sm border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-400 focus:border-transparent"
              placeholder="Optional alias"
            />
          </div>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {/* Service slots */}
        {descriptor?.service_deps && Object.keys(descriptor.service_deps).length > 0 && (
          <div>
            <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-2">
              Services
            </h4>
            {Object.entries(descriptor.service_deps).map(([slot, dep]) => (
              <ServiceSlotWidget
                key={slot}
                slot={slot}
                prefix={dep.prefix}
                required={dep.required}
                value={node.services?.[slot] ?? ""}
                onChange={(value) => onServicesChange(slot, value)}
              />
            ))}
          </div>
        )}

        {/* Config form */}
        {loading ? (
          <div className="text-sm text-gray-400">Loading schema...</div>
        ) : schema ? (
          <div>
            <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-2">
              Configuration
            </h4>
            <Form
              schema={schema}
              uiSchema={uiSchema}
              formData={node.config ?? {}}
              validator={validator}
              onChange={onConfigChange}
              widgets={widgets}
              fields={fields}
              templates={{ ObjectFieldTemplate: StyledObjectFieldTemplate }}
              liveValidate
              showErrorList={false}
            >
              {/* Hide submit button */}
              <></>
            </Form>
          </div>
        ) : (
          <div className="text-sm text-gray-400">
            No configurable fields for this node type.
          </div>
        )}

        {/* Raw JSON preview */}
        <div>
          <h4 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-2">
            JSON Preview
          </h4>
          <pre className="p-3 bg-gray-50 rounded text-xs text-gray-600 overflow-x-auto whitespace-pre-wrap border border-gray-200">
            {JSON.stringify(
              { id: node.id, type: node.type, config: node.config, services: node.services },
              null,
              2
            )}
          </pre>
        </div>
      </div>
    </div>
  );
}

function SaveIndicator({ status }: { status: string }) {
  if (status === "idle") return null;
  const styles: Record<string, string> = {
    saving: "text-blue-500",
    saved: "text-green-500",
    error: "text-red-500",
  };
  const labels: Record<string, string> = {
    saving: "Saving...",
    saved: "Saved",
    error: "Save error",
  };
  return (
    <span className={`text-xs ${styles[status] ?? ""}`}>
      {labels[status] ?? ""}
    </span>
  );
}
