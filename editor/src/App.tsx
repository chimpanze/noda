import { useEffect } from "react";
import { ReactFlowProvider } from "@xyflow/react";
import { Sidebar } from "@/components/layout/Sidebar";
import { Toolbar } from "@/components/layout/Toolbar";
import { WorkflowList } from "@/components/layout/WorkflowList";
import { WorkflowCanvas } from "@/components/canvas/WorkflowCanvas";
import { NodePalette } from "@/components/canvas/NodePalette";
import { NodeConfigPanel } from "@/components/panels/NodeConfigPanel";
import { TracePanel } from "@/components/panels/TracePanel";
import { RoutesView } from "@/components/views/RoutesView";
import { ServicesView } from "@/components/views/ServicesView";
import { SchemasView } from "@/components/views/SchemasView";
import { TestsView } from "@/components/views/TestsView";
import { ShortcutModal } from "@/components/panels/ShortcutModal";
import { ToastContainer } from "@/components/panels/Toast";
import { ConnectionOverlay } from "@/components/panels/ConnectionOverlay";
import { ValidationSummary } from "@/components/panels/ValidationSummary";
import { useEditorStore } from "@/stores/editor";
import { useKeyboardShortcuts } from "@/hooks/useKeyboardShortcuts";
import { useValidation } from "@/hooks/useValidation";
import { connectTrace, disconnectTrace } from "@/api/traceClient";

export default function App() {
  const activeView = useEditorStore((s) => s.activeView);
  const loadFiles = useEditorStore((s) => s.loadFiles);
  const loadNodeTypes = useEditorStore((s) => s.loadNodeTypes);
  const selectedNodeId = useEditorStore((s) => s.selectedNodeId);
  const activeWorkflow = useEditorStore((s) => s.activeWorkflow);

  useEffect(() => {
    loadFiles();
    loadNodeTypes();
    connectTrace();
    return () => disconnectTrace();
  }, [loadFiles, loadNodeTypes]);

  const { showShortcuts, closeShortcuts } = useKeyboardShortcuts();
  useValidation();

  return (
    <ReactFlowProvider>
      <div className="flex h-full">
        {/* Left sidebar - navigation */}
        <Sidebar />

        {/* Main content area */}
        <div className="flex-1 flex flex-col min-w-0">
          {/* Toolbar */}
          <Toolbar />

          <div className="flex-1 flex min-h-0">
            {activeView === "workflows" ? (
              <>
                {/* Workflow list */}
                <WorkflowList />

                {/* Node palette (when a workflow is open) */}
                {activeWorkflow && <NodePalette />}

                {/* Canvas */}
                <WorkflowCanvas />

                {/* Right panel - node config */}
                {selectedNodeId && (
                  <div className="w-80 border-l border-gray-200 bg-white overflow-hidden">
                    <NodeConfigPanel />
                  </div>
                )}
              </>
            ) : activeView === "routes" ? (
              <RoutesView />
            ) : activeView === "services" ? (
              <ServicesView />
            ) : activeView === "schemas" ? (
              <SchemasView />
            ) : activeView === "tests" ? (
              <TestsView />
            ) : (
              <div className="flex-1 flex items-center justify-center text-gray-400 text-sm">
                {activeView.charAt(0).toUpperCase() + activeView.slice(1)} view
                — coming in future milestones.
              </div>
            )}
          </div>

          {/* Validation summary */}
          <ValidationSummary />

          {/* Bottom trace panel */}
          <div className="h-44 border-t border-gray-200 bg-white">
            <TracePanel />
          </div>
        </div>
      </div>
      {showShortcuts && <ShortcutModal onClose={closeShortcuts} />}
      <ConnectionOverlay />
      <ToastContainer />
    </ReactFlowProvider>
  );
}
