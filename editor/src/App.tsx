import { useEffect } from "react";
import { ReactFlowProvider } from "@xyflow/react";
import { Sidebar } from "@/components/layout/Sidebar";
import { WorkflowList } from "@/components/layout/WorkflowList";
import { WorkflowCanvas } from "@/components/canvas/WorkflowCanvas";
import { NodeDetail } from "@/components/panels/NodeDetail";
import { DebugPanel } from "@/components/panels/DebugPanel";
import { useEditorStore } from "@/stores/editor";

export default function App() {
  const activeView = useEditorStore((s) => s.activeView);
  const loadFiles = useEditorStore((s) => s.loadFiles);
  const loadNodeTypes = useEditorStore((s) => s.loadNodeTypes);
  const selectedNodeId = useEditorStore((s) => s.selectedNodeId);

  useEffect(() => {
    loadFiles();
    loadNodeTypes();
  }, [loadFiles, loadNodeTypes]);

  return (
    <ReactFlowProvider>
      <div className="flex h-full">
        {/* Left sidebar - navigation */}
        <Sidebar />

        {/* Main content area */}
        <div className="flex-1 flex flex-col min-w-0">
          <div className="flex-1 flex min-h-0">
            {activeView === "workflows" ? (
              <>
                {/* Workflow list panel */}
                <WorkflowList />

                {/* Canvas */}
                <WorkflowCanvas />

                {/* Right panel - node detail */}
                {selectedNodeId && (
                  <div className="w-72 border-l border-gray-200 bg-white overflow-hidden">
                    <NodeDetail />
                  </div>
                )}
              </>
            ) : (
              <div className="flex-1 flex items-center justify-center text-gray-400 text-sm">
                {activeView.charAt(0).toUpperCase() + activeView.slice(1)} view
                — coming in future milestones.
              </div>
            )}
          </div>

          {/* Bottom debug panel */}
          <div className="h-36 border-t border-gray-200 bg-white">
            <DebugPanel />
          </div>
        </div>
      </div>
    </ReactFlowProvider>
  );
}
