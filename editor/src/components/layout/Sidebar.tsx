import {
  GitBranch,
  Route,
  Shield,
  Radio,
  Clock,
  Cable,
  Server,
  FileJson,
  Cpu,
  TestTube,
  Settings,
  BookOpen,
} from "lucide-react";
import { useEditorStore } from "@/stores/editor";
import type { ViewType } from "@/types";

interface NavItem {
  view: ViewType;
  label: string;
  icon: React.ReactNode;
}

interface NavGroup {
  label: string;
  items: NavItem[];
}

const navGroups: NavGroup[] = [
  {
    label: "HTTP",
    items: [
      { view: "routes", label: "Routes", icon: <Route size={18} /> },
      { view: "middleware", label: "Middleware", icon: <Shield size={18} /> },
    ],
  },
  {
    label: "Logic",
    items: [
      { view: "workflows", label: "Workflows", icon: <GitBranch size={18} /> },
      { view: "workers", label: "Workers", icon: <Radio size={18} /> },
      { view: "schedules", label: "Schedules", icon: <Clock size={18} /> },
    ],
  },
  {
    label: "Realtime",
    items: [
      { view: "connections", label: "Connections", icon: <Cable size={18} /> },
      { view: "wasm", label: "Wasm", icon: <Cpu size={18} /> },
    ],
  },
  {
    label: "Data",
    items: [
      { view: "services", label: "Services", icon: <Server size={18} /> },
      { view: "schemas", label: "Schemas", icon: <FileJson size={18} /> },
    ],
  },
  {
    label: "Dev",
    items: [
      { view: "tests", label: "Tests", icon: <TestTube size={18} /> },
      { view: "docs", label: "Docs", icon: <BookOpen size={18} /> },
    ],
  },
  {
    label: "System",
    items: [
      { view: "settings", label: "Settings", icon: <Settings size={18} /> },
    ],
  },
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
      <nav className="flex-1 py-1 overflow-y-auto">
        {navGroups.map((group) => (
          <div key={group.label}>
            <div className="px-4 pt-3 pb-1 text-[10px] font-semibold text-gray-400 uppercase tracking-wider">
              {group.label}
            </div>
            {group.items.map((item) => (
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
          </div>
        ))}
      </nav>
    </aside>
  );
}
