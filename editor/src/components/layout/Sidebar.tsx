import {
  GitBranch,
  Route,
  Radio,
  Clock,
  Cable,
  Server,
  FileJson,
  Cpu,
  TestTube,
  Database,
} from "lucide-react";
import { useEditorStore } from "@/stores/editor";
import type { ViewType } from "@/types";

const navItems: { view: ViewType; label: string; icon: React.ReactNode }[] = [
  { view: "workflows", label: "Workflows", icon: <GitBranch size={18} /> },
  { view: "routes", label: "Routes", icon: <Route size={18} /> },
  { view: "workers", label: "Workers", icon: <Radio size={18} /> },
  { view: "schedules", label: "Schedules", icon: <Clock size={18} /> },
  { view: "connections", label: "Connections", icon: <Cable size={18} /> },
  { view: "services", label: "Services", icon: <Server size={18} /> },
  { view: "schemas", label: "Schemas", icon: <FileJson size={18} /> },
  { view: "wasm", label: "Wasm", icon: <Cpu size={18} /> },
  { view: "tests", label: "Tests", icon: <TestTube size={18} /> },
  { view: "migrations", label: "Migrations", icon: <Database size={18} /> },
];

export function Sidebar() {
  const activeView = useEditorStore((s) => s.activeView);
  const setActiveView = useEditorStore((s) => s.setActiveView);

  return (
    <aside className="w-56 bg-gray-50 border-r border-gray-200 flex flex-col shrink-0">
      <div className="px-4 py-3 border-b border-gray-200">
        <h1 className="text-lg font-semibold text-gray-900">Noda</h1>
        <p className="text-xs text-gray-500">Visual Editor</p>
      </div>
      <nav className="flex-1 py-2 overflow-y-auto">
        {navItems.map((item) => (
          <button
            key={item.view}
            onClick={() => setActiveView(item.view)}
            className={`w-full flex items-center gap-3 px-4 py-2 text-sm transition-colors ${
              activeView === item.view
                ? "bg-blue-50 text-blue-700 font-medium"
                : "text-gray-600 hover:bg-gray-100 hover:text-gray-900"
            }`}
          >
            {item.icon}
            {item.label}
          </button>
        ))}
      </nav>
    </aside>
  );
}
