import { useEffect, useState, useCallback } from "react";
import Form, { type IChangeEvent } from "@rjsf/core";
import validator from "@rjsf/validator-ajv8";
import type { RJSFSchema, UiSchema } from "@rjsf/utils";
import { useEditorStore } from "@/stores/editor";
import * as api from "@/api/client";
import { ExpressionWidget } from "./ExpressionWidget";
import { ServiceSlotWidget } from "./ServiceSlotWidget";

// Custom widget registry for RJSF
const widgets = {
  expression: ExpressionWidget,
};

export function NodeConfigPanel() {
  const selectedNodeId = useEditorStore((s) => s.selectedNodeId);
  const activeWorkflow = useEditorStore((s) => s.activeWorkflow);
  const updateNodeConfig = useEditorStore((s) => s.updateNodeConfig);
  const updateNodeServices = useEditorStore((s) => s.updateNodeServices);
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

  // Build uiSchema — mark string fields as expression widgets
  const uiSchema: UiSchema = {};
  if (schema?.properties) {
    for (const [key, prop] of Object.entries(schema.properties)) {
      const p = prop as Record<string, unknown>;
      if (p.type === "string") {
        uiSchema[key] = { "ui:widget": "expression" };
      }
    }
  }

  return (
    <div className="flex flex-col h-full overflow-hidden">
      {/* Header */}
      <div className="px-4 py-3 border-b border-gray-200 shrink-0">
        <div className="flex items-center justify-between">
          <div>
            <div className="text-xs font-mono text-gray-400">{node.type}</div>
            <div className="text-sm font-semibold text-gray-900">
              {node.as ?? node.id}
            </div>
          </div>
          <SaveIndicator status={saveStatus} />
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
