import axios from "axios";
import type {
  FileList,
  NodeDescriptor,
  ServiceInfo,
  PluginInfo,
  SchemaInfo,
  ValidationResult,
} from "@/types";

const api = axios.create({
  baseURL: "/_noda",
});

// File operations
export async function listFiles(): Promise<FileList> {
  const { data } = await api.get<FileList>("/files");
  return data;
}

export async function readFile(path: string): Promise<unknown> {
  const { data } = await api.get(`/files/${encodeURIComponent(path)}`);
  return data;
}

export async function writeFile(
  path: string,
  content: unknown
): Promise<void> {
  await api.put(`/files/${encodeURIComponent(path)}`, content);
}

export async function deleteFile(path: string): Promise<void> {
  await api.delete(`/files/${encodeURIComponent(path)}`);
}

// Validation
export async function validateFile(
  path: string,
  content?: unknown
): Promise<ValidationResult> {
  const { data } = await api.post<ValidationResult>("/validate", {
    path,
    content,
  });
  return data;
}

export async function validateAll(): Promise<ValidationResult> {
  const { data } = await api.post<ValidationResult>("/validate/all");
  return data;
}

// Node registry
export async function listNodes(): Promise<NodeDescriptor[]> {
  const { data } = await api.get<{ nodes: NodeDescriptor[] }>("/nodes");
  return data.nodes;
}

export async function getNodeSchema(
  type: string
): Promise<Record<string, unknown>> {
  const { data } = await api.get<Record<string, unknown>>(
    `/nodes/${encodeURIComponent(type)}/schema`
  );
  return data;
}

export async function computeNodeOutputs(
  type: string,
  config?: Record<string, unknown>
): Promise<string[]> {
  const { data } = await api.post<{ outputs: string[] }>(
    `/nodes/${encodeURIComponent(type)}/outputs`,
    config ?? {}
  );
  return data.outputs;
}

// Expression tools
export interface ExpressionValidation {
  valid: boolean;
  error?: string;
}

export interface ExpressionContextVar {
  name: string;
  type: string;
  description: string;
}

export interface ExpressionContext {
  variables: ExpressionContextVar[];
  functions: ExpressionContextVar[];
  upstream: { node_id: string; node_type: string; ref: string }[];
}

export async function validateExpression(
  expression: string
): Promise<ExpressionValidation> {
  const { data } = await api.post<ExpressionValidation>(
    "/expressions/validate",
    { expression }
  );
  return data;
}

export async function getExpressionContext(
  workflow: string,
  node?: string
): Promise<ExpressionContext> {
  const params = new URLSearchParams({ workflow });
  if (node) params.set("node", node);
  const { data } = await api.get<ExpressionContext>(
    `/expressions/context?${params}`
  );
  return data;
}

// Services and plugins
export async function listServices(): Promise<ServiceInfo[]> {
  const { data } = await api.get<{ services: ServiceInfo[] }>("/services");
  return data.services;
}

export async function listPlugins(): Promise<PluginInfo[]> {
  const { data } = await api.get<{ plugins: PluginInfo[] }>("/plugins");
  return data.plugins;
}

export async function listSchemas(): Promise<SchemaInfo[]> {
  const { data } = await api.get<{ schemas: SchemaInfo[] }>("/schemas");
  return data.schemas;
}
