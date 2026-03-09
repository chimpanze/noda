// Noda editor types

export interface FileList {
  root: string;
  overlay: string;
  schemas: string[];
  routes: string[];
  workflows: string[];
  workers: string[];
  schedules: string[];
  connections: string[];
  tests: string[];
}

export interface NodeDescriptor {
  type: string;
  name: string;
  outputs: string[];
  has_schema?: boolean;
  service_deps?: Record<string, { prefix: string; required: boolean }>;
}

export interface ServiceInfo {
  name: string;
  prefix: string;
  health: "healthy" | "unhealthy" | "unknown";
  error?: string;
}

export interface PluginInfo {
  name: string;
  prefix: string;
  has_services: boolean;
  node_count: number;
}

export interface SchemaInfo {
  path: string;
  schema: Record<string, unknown>;
}

export interface ValidationError {
  file: string;
  path: string;
  message: string;
}

export interface ValidationResult {
  valid: boolean;
  errors: ValidationError[];
}

// Workflow config types (matching Noda JSON format)
export interface WorkflowConfig {
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
}

export interface WorkflowNode {
  id: string;
  type: string;
  config?: Record<string, unknown>;
  as?: string;
  services?: Record<string, string>;
  position?: { x: number; y: number };
}

export interface WorkflowEdge {
  from: string;
  output: string;
  to: string;
  retry?: RetryConfig;
}

export interface RetryConfig {
  attempts: number;
  delay: string;
  backoff?: string;
}

// Trace events
export interface TraceEvent {
  type: string;
  workflow_id: string;
  trace_id: string;
  timestamp?: string;
  [key: string]: unknown;
}

export type ViewType =
  | "workflows"
  | "routes"
  | "workers"
  | "schedules"
  | "connections"
  | "services"
  | "schemas"
  | "wasm"
  | "tests"
  | "migrations";
