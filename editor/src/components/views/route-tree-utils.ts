export const methodColors: Record<string, string> = {
  GET: "bg-green-100 text-green-800",
  POST: "bg-blue-100 text-blue-800",
  PUT: "bg-yellow-100 text-yellow-800",
  PATCH: "bg-orange-100 text-orange-800",
  DELETE: "bg-red-100 text-red-800",
  HEAD: "bg-gray-100 text-gray-700",
  OPTIONS: "bg-purple-100 text-purple-800",
};

export interface RouteFileEntry {
  filePath: string;
  route: { id: string; method: string; path: string; summary?: string; trigger?: { workflow: string }; [key: string]: unknown };
}

export interface TreeNode {
  segment: string;
  fullPath: string;
  children: Map<string, TreeNode>;
  routes: RouteFileEntry[];
}

export function buildTree(entries: RouteFileEntry[]): TreeNode {
  const root: TreeNode = {
    segment: "",
    fullPath: "",
    children: new Map(),
    routes: [],
  };

  for (const entry of entries) {
    const parts = entry.route.path.split("/").filter(Boolean);
    let node = root;
    let path = "";
    for (const part of parts) {
      path += "/" + part;
      if (!node.children.has(part)) {
        node.children.set(part, {
          segment: part,
          fullPath: path,
          children: new Map(),
          routes: [],
        });
      }
      node = node.children.get(part)!;
    }
    node.routes.push(entry);
  }

  return root;
}

export function collapseTree(node: TreeNode): TreeNode {
  const collapsedChildren = new Map<string, TreeNode>();
  for (const [key, child] of node.children) {
    collapsedChildren.set(key, collapseTree(child));
  }
  node.children = collapsedChildren;

  if (
    node.segment !== "" &&
    node.children.size === 1 &&
    node.routes.length === 0
  ) {
    const [, child] = [...node.children.entries()][0];
    return {
      segment: node.segment
        ? node.segment + "/" + child.segment
        : child.segment,
      fullPath: child.fullPath,
      children: child.children,
      routes: child.routes,
    };
  }

  return node;
}

export function countRoutes(node: TreeNode): number {
  let count = node.routes.length;
  for (const child of node.children.values()) {
    count += countRoutes(child);
  }
  return count;
}
