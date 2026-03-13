// Node category colors and icons for the workflow canvas.

export interface CategoryStyle {
  bg: string;
  border: string;
  text: string;
  iconColor: string;
}

const categoryStyles: Record<string, CategoryStyle> = {
  control: {
    bg: "bg-purple-50",
    border: "border-purple-300",
    text: "text-purple-800",
    iconColor: "text-purple-500",
  },
  workflow: {
    bg: "bg-blue-50",
    border: "border-blue-300",
    text: "text-blue-800",
    iconColor: "text-blue-500",
  },
  transform: {
    bg: "bg-yellow-50",
    border: "border-yellow-300",
    text: "text-yellow-800",
    iconColor: "text-yellow-600",
  },
  response: {
    bg: "bg-green-50",
    border: "border-green-300",
    text: "text-green-800",
    iconColor: "text-green-500",
  },
  util: {
    bg: "bg-gray-50",
    border: "border-gray-300",
    text: "text-gray-700",
    iconColor: "text-gray-500",
  },
  db: {
    bg: "bg-orange-50",
    border: "border-orange-300",
    text: "text-orange-800",
    iconColor: "text-orange-500",
  },
  cache: {
    bg: "bg-cyan-50",
    border: "border-cyan-300",
    text: "text-cyan-800",
    iconColor: "text-cyan-500",
  },
  storage: {
    bg: "bg-teal-50",
    border: "border-teal-300",
    text: "text-teal-800",
    iconColor: "text-teal-500",
  },
  image: {
    bg: "bg-pink-50",
    border: "border-pink-300",
    text: "text-pink-800",
    iconColor: "text-pink-500",
  },
  http: {
    bg: "bg-indigo-50",
    border: "border-indigo-300",
    text: "text-indigo-800",
    iconColor: "text-indigo-500",
  },
  email: {
    bg: "bg-red-50",
    border: "border-red-300",
    text: "text-red-800",
    iconColor: "text-red-500",
  },
  event: {
    bg: "bg-amber-50",
    border: "border-amber-300",
    text: "text-amber-800",
    iconColor: "text-amber-500",
  },
  ws: {
    bg: "bg-violet-50",
    border: "border-violet-300",
    text: "text-violet-800",
    iconColor: "text-violet-500",
  },
  sse: {
    bg: "bg-violet-50",
    border: "border-violet-300",
    text: "text-violet-800",
    iconColor: "text-violet-500",
  },
  upload: {
    bg: "bg-stone-50",
    border: "border-stone-300",
    text: "text-stone-800",
    iconColor: "text-stone-500",
  },
  wasm: {
    bg: "bg-emerald-50",
    border: "border-emerald-300",
    text: "text-emerald-800",
    iconColor: "text-emerald-500",
  },
};

const defaultStyle: CategoryStyle = {
  bg: "bg-gray-50",
  border: "border-gray-300",
  text: "text-gray-700",
  iconColor: "text-gray-500",
};

export function getCategoryStyle(nodeType: string): CategoryStyle {
  const prefix = nodeType.split(".")[0];
  return categoryStyles[prefix] ?? defaultStyle;
}

// Output port colors
export function getOutputColor(output: string): string {
  switch (output) {
    case "success":
    case "done":
      return "#22c55e";
    case "error":
      return "#ef4444";
    case "then":
      return "#3b82f6";
    case "else":
      return "#f97316";
    case "default":
      return "#9ca3af";
    default:
      return "#6366f1";
  }
}
