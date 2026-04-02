import axios from "axios";
import type {
  FileList,
  NodeDescriptor,
  ServiceInfo,
  PluginInfo,
  SchemaInfo,
  MiddlewareInfo,
  ModelInfo,
  VarInfo,
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

export async function writeFile(path: string, content: unknown): Promise<void> {
  await api.put(`/files/${encodeURIComponent(path)}`, content);
}

export async function deleteFile(path: string): Promise<void> {
  await api.delete(`/files/${encodeURIComponent(path)}`);
}

// Validation
export async function validateFile(
  path: string,
  content?: unknown,
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
  type: string,
): Promise<Record<string, unknown>> {
  const { data } = await api.get<Record<string, unknown>>(
    `/nodes/${encodeURIComponent(type)}/schema`,
  );
  return data;
}

export async function computeNodeOutputs(
  type: string,
  config?: Record<string, unknown>,
): Promise<string[]> {
  const { data } = await api.post<{ outputs: string[] }>(
    `/nodes/${encodeURIComponent(type)}/outputs`,
    config ?? {},
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
  expression: string,
): Promise<ExpressionValidation> {
  const { data } = await api.post<ExpressionValidation>(
    "/expressions/validate",
    { expression },
  );
  return data;
}

export async function getExpressionContext(
  workflow: string,
  node?: string,
): Promise<ExpressionContext> {
  const params = new URLSearchParams({ workflow });
  if (node) params.set("node", node);
  const { data } = await api.get<ExpressionContext>(
    `/expressions/context?${params}`,
  );
  return data;
}

// Output schemas
export async function fetchOutputSchemas(): Promise<Record<string, any>> {
  const { data } = await api.get<{ schemas: Record<string, any> }>(
    "/schemas/output",
  );
  return data.schemas;
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

export async function listMiddleware(): Promise<MiddlewareInfo> {
  const { data } = await api.get<MiddlewareInfo>("/middleware");
  return data;
}

// Models
export async function listModels(): Promise<ModelInfo[]> {
  const { data } = await api.get<{ models: ModelInfo[] }>("/models");
  return data.models;
}

export interface MigrationPreview {
  status: string;
  up: string;
  down: string;
  up_path?: string;
  down_path?: string;
}

export async function generateMigration(
  confirm: boolean = false,
): Promise<MigrationPreview> {
  const { data } = await api.post<MigrationPreview>(
    "/models/generate-migration",
    { confirm },
  );
  return data;
}

export interface CRUDPreview {
  status: string;
  files: Record<string, unknown>;
}

export async function generateCRUD(opts: {
  model: string;
  confirm?: boolean;
  service?: string;
  base_path?: string;
  operations?: string[];
  artifacts?: string[];
  scope_column?: string;
  scope_param?: string;
}): Promise<CRUDPreview> {
  const { data } = await api.post<CRUDPreview>("/models/generate-crud", opts);
  return data;
}

// Shared variables
export async function listVars(): Promise<VarInfo[]> {
  const { data } = await api.get<{ variables: VarInfo[] }>("/vars");
  return data.variables;
}

// Environment variables
export interface EnvVarInfo {
  name: string;
  defined: boolean;
  sources: string[];
}

export async function getEnvVars(): Promise<EnvVarInfo[]> {
  const { data } = await api.get<{ variables: EnvVarInfo[] }>("/env");
  return data.variables;
}

// OpenAPI
export async function getOpenAPISpec(): Promise<Record<string, unknown>> {
  const { data } = await api.get<Record<string, unknown>>("/openapi");
  return data;
}

// Test runner
export interface TestRunResult {
  case_name: string;
  passed: boolean;
  error?: string;
  duration: string;
  expected: {
    status: string;
    output: Record<string, unknown>;
    error_node: string;
  };
  actual: {
    status: string;
    outputs: Record<string, unknown>;
    error_node?: string;
    error_msg?: string;
  };
}

export async function runTests(suitePath: string): Promise<TestRunResult[]> {
  const { data } = await api.post<TestRunResult[]>("/tests/run", {
    path: suitePath,
  });
  return data;
}
