import { ExternalLink, ChevronRight, ChevronDown, Shield } from "lucide-react";
import type { RouteGroupConfig } from "@/types";
import {
  methodColors,
  countRoutes,
  type RouteFileEntry,
  type TreeNode,
} from "./route-tree-utils";

export function TreeNodeView({
  node,
  depth,
  expanded,
  toggleExpand,
  selectedIndex,
  isNew,
  routeEntries,
  selectRoute,
  goToWorkflow,
  routeGroups,
  selectedGroup,
  onSelectGroup,
}: {
  node: TreeNode;
  depth: number;
  expanded: Set<string>;
  toggleExpand: (path: string) => void;
  selectedIndex: number | null;
  isNew: boolean;
  routeEntries: RouteFileEntry[];
  selectRoute: (index: number) => void;
  goToWorkflow: (workflowId: string) => void;
  routeGroups: Record<string, RouteGroupConfig>;
  selectedGroup: string | null;
  onSelectGroup: (fullPath: string) => void;
}) {
  const hasChildren = node.children.size > 0 || node.routes.length > 0;
  const isExpanded = expanded.has(node.fullPath);
  const isGroup = node.children.size > 0 || node.routes.length > 1;

  if (!isGroup && node.routes.length === 1) {
    const entry = node.routes[0];
    const idx = routeEntries.indexOf(entry);
    return (
      <RouteItem
        key={`${entry.filePath}-${entry.route.id}`}
        entry={entry}
        index={idx}
        selected={selectedIndex === idx && !isNew}
        onSelect={selectRoute}
        goToWorkflow={goToWorkflow}
        indent={depth}
      />
    );
  }

  const hasGroup = !!routeGroups[node.fullPath];
  const isGroupSelected = selectedGroup === node.fullPath;

  return (
    <div>
      <div
        className={`w-full flex items-center gap-1.5 px-4 py-1.5 text-xs font-medium text-gray-500 hover:bg-gray-50 ${
          isGroupSelected ? "bg-blue-50" : ""
        }`}
        style={{ paddingLeft: `${16 + depth * 12}px` }}
      >
        <button
          onClick={() => toggleExpand(node.fullPath)}
          className="shrink-0"
        >
          {hasChildren ? (
            isExpanded ? (
              <ChevronDown size={12} />
            ) : (
              <ChevronRight size={12} />
            )
          ) : null}
        </button>
        <button
          onClick={() => onSelectGroup(node.fullPath)}
          className="flex items-center gap-1.5 min-w-0"
        >
          <span className="font-mono text-gray-600">/{node.segment}</span>
          {hasGroup && <Shield size={12} className="text-blue-500 shrink-0" />}
          <span className="text-gray-400 ml-1">({countRoutes(node)})</span>
        </button>
      </div>

      {isExpanded && (
        <>
          {node.routes.map((entry) => {
            const idx = routeEntries.indexOf(entry);
            return (
              <RouteItem
                key={`${entry.filePath}-${entry.route.id}`}
                entry={entry}
                index={idx}
                selected={selectedIndex === idx && !isNew}
                onSelect={selectRoute}
                goToWorkflow={goToWorkflow}
                indent={depth + 1}
              />
            );
          })}
          {[...node.children.values()].map((child) => (
            <TreeNodeView
              key={child.fullPath}
              node={child}
              depth={depth + 1}
              expanded={expanded}
              toggleExpand={toggleExpand}
              selectedIndex={selectedIndex}
              isNew={isNew}
              routeEntries={routeEntries}
              selectRoute={selectRoute}
              goToWorkflow={goToWorkflow}
              routeGroups={routeGroups}
              selectedGroup={selectedGroup}
              onSelectGroup={onSelectGroup}
            />
          ))}
        </>
      )}
    </div>
  );
}

export function RouteItem({
  entry,
  index,
  selected,
  onSelect,
  goToWorkflow,
  indent = 0,
}: {
  entry: RouteFileEntry;
  index: number;
  selected: boolean;
  onSelect: (index: number) => void;
  goToWorkflow: (workflowId: string) => void;
  indent?: number;
}) {
  return (
    <button
      onClick={() => onSelect(index)}
      className={`w-full text-left px-4 py-2.5 hover:bg-gray-50 flex items-center gap-3 border-b border-gray-100 ${
        selected ? "bg-blue-50" : ""
      }`}
      style={{ paddingLeft: `${16 + indent * 12}px` }}
    >
      <span
        className={`px-2 py-0.5 text-xs font-mono font-medium rounded shrink-0 ${
          methodColors[entry.route.method] ?? "bg-gray-100 text-gray-700"
        }`}
      >
        {entry.route.method}
      </span>
      <div className="flex-1 min-w-0">
        <div className="text-sm font-medium text-gray-800 truncate">
          {entry.route.path}
        </div>
        {entry.route.summary && (
          <div className="text-xs text-gray-400 truncate">
            {entry.route.summary}
          </div>
        )}
      </div>
      {entry.route.trigger?.workflow && (
        <button
          onClick={(e) => {
            e.stopPropagation();
            goToWorkflow(entry.route.trigger!.workflow);
          }}
          className="text-blue-400 hover:text-blue-600 shrink-0"
          title="Open workflow"
        >
          <ExternalLink size={12} />
        </button>
      )}
    </button>
  );
}
