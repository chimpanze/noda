declare module "virtual:docs" {
  interface DocEntry {
    path: string;
    title: string;
    group: string;
    content: string;
    nodeType?: string;
    sortOrder?: number;
  }
  export const docs: DocEntry[];
  export const nodeDocIndex: Record<string, string>;
}
