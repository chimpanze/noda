declare module "virtual:docs" {
  interface DocEntry {
    path: string;
    title: string;
    group: string;
    content: string;
  }
  export const docs: DocEntry[];
}
