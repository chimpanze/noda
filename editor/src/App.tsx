import { useEffect } from "react";
import { ReactFlowProvider } from "@xyflow/react";
import { Sidebar } from "@/components/layout/Sidebar";
import { Toolbar } from "@/components/layout/Toolbar";
import { WorkflowList } from "@/components/layout/WorkflowList";
import { WorkflowCanvas } from "@/components/canvas/WorkflowCanvas";
import { WorkflowTabs } from "@/components/layout/WorkflowTabs";
import { NodePalette } from "@/components/canvas/NodePalette";
import { NodeConfigPanel } from "@/components/panels/NodeConfigPanel";
import { EdgeConfigPanel } from "@/components/panels/EdgeConfigPanel";
import { TracePanel } from "@/components/panels/TracePanel";
import { RoutesView } from "@/components/views/RoutesView";
import { ServicesView } from "@/components/views/ServicesView";
import { SchemasView } from "@/components/views/SchemasView";
import { TestsView } from "@/components/views/TestsView";
import { WorkersView } from "@/components/views/WorkersView";
import { SchedulesView } from "@/components/views/SchedulesView";
import { ConnectionsView } from "@/components/views/ConnectionsView";
import { WasmView } from "@/components/views/WasmView";
import { MiddlewareView } from "@/components/views/MiddlewareView";
import { ServerSettingsView } from "@/components/views/ServerSettingsView";
import { DocsView } from "@/components/views/DocsView";
import { ShortcutModal } from "@/components/panels/ShortcutModal";
import { CommandPalette } from "@/components/panels/CommandPalette";
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
  const selectedEdgeIndex = useEditorStore((s) => s.selectedEdgeIndex);
  const activeWorkflow = useEditorStore((s) => s.activeWorkflow);

  const dirtyFiles = useEditorStore((s) => s.dirtyFiles);

  useEffect(() => {
    loadFiles();
    loadNodeTypes();
    connectTrace();
    return () => disconnectTrace();
  }, [loadFiles, loadNodeTypes]);

  // Warn before navigating away with unsaved changes
  useEffect(() => {
    const handler = (e: BeforeUnloadEvent) => {
      if (dirtyFiles.size > 0) {
        e.preventDefault();
      }
    };
    window.addEventListener("beforeunload", handler);
    return () => window.removeEventListener("beforeunload", handler);
  }, [dirtyFiles]);

  const { showShortcuts, closeShortcuts, showCommandPalette, closeCommandPalette } = useKeyboardShortcuts();
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

                {/* Canvas area with tabs */}
                <div className="flex-1 flex flex-col min-w-0">
                  {/* Workflow tabs */}
                  <WorkflowTabs />

                  <div className="flex-1 flex min-h-0">
                    {/* Node palette (when a workflow is open) */}
                    {activeWorkflow && <NodePalette />}

                    {/* Canvas */}
                    <WorkflowCanvas />
                  </div>
                </div>

                {/* Right panel - node/edge config */}
                {(selectedNodeId || selectedEdgeIndex !== null) && (
                  <div className="w-80 border-l border-gray-200 bg-white overflow-hidden">
                    {selectedNodeId ? <NodeConfigPanel /> : <EdgeConfigPanel />}
                  </div>
                )}
              </>
            ) : activeView === "routes" ? (
              <RoutesView />
            ) : activeView === "middleware" ? (
              <MiddlewareView />
            ) : activeView === "services" ? (
              <ServicesView />
            ) : activeView === "schemas" ? (
              <SchemasView />
            ) : activeView === "tests" ? (
              <TestsView />
            ) : activeView === "workers" ? (
              <WorkersView />
            ) : activeView === "schedules" ? (
              <SchedulesView />
            ) : activeView === "connections" ? (
              <ConnectionsView />
            ) : activeView === "wasm" ? (
              <WasmView />
            ) : activeView === "settings" ? (
              <ServerSettingsView />
            ) : activeView === "docs" ? (
              <DocsView />
            ) : null}
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
      {showCommandPalette && <CommandPalette onClose={closeCommandPalette} />}
      <ConnectionOverlay />
      <ToastContainer />
    </ReactFlowProvider>
  );
}
