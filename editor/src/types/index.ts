// Noda editor types

export interface FileList {
  root: string;
  overlay: string;
  vars: string;
  schemas: string[];
  routes: string[];
  workflows: string[];
  workers: string[];
  schedules: string[];
  connections: string[];
  tests: string[];
  models: string[];
}

export interface NodeDescriptor {
  type: string;
  name: string;
  description?: string;
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
  description?: string;
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

// Trace events (matching Go internal/trace/events.go)
export type TraceEventType =
  | "workflow:started"
  | "workflow:completed"
  | "workflow:failed"
  | "node:entered"
  | "node:completed"
  | "node:failed"
  | "edge:followed"
  | "retry:attempted";

export interface TraceEvent {
  type: TraceEventType;
  timestamp: string;
  trace_id: string;
  workflow_id: string;
  node_id?: string;
  node_type?: string;
  output?: string;
  duration?: string;
  error?: string;
  from_node?: string;
  to_node?: string;
  data?: unknown;
}

export type NodeExecState = "idle" | "running" | "completed" | "failed";

export interface Execution {
  traceId: string;
  workflowId: string;
  status: "running" | "completed" | "failed";
  startedAt: string;
  duration?: string;
  events: TraceEvent[];
  nodeStates: Map<string, NodeExecState>;
  nodeData: Map<string, { output?: string; data?: unknown; error?: string; duration?: string }>;
}

export interface RouteGroupConfig {
  middleware_preset?: string;
  middleware?: string[];
  tags?: string[];
}

export type ViewType =
  | "workflows"
  | "routes"
  | "middleware"
  | "workers"
  | "schedules"
  | "connections"
  | "services"
  | "schemas"
  | "vars"
  | "wasm"
  | "tests"
  | "models"
  | "settings"
  | "docs";

export interface VarInfo {
  name: string;
  value: string;
  usages: string[];
}

// Model types
export interface ModelInfo {
  path: string;
  model: ModelDefinition;
}

export interface ModelDefinition {
  table: string;
  columns: Record<string, ColumnDef>;
  relations?: Record<string, RelationDef>;
  indexes?: IndexDef[];
  timestamps?: boolean;
  soft_delete?: boolean;
}

export interface ColumnDef {
  type: string;
  primary_key?: boolean;
  not_null?: boolean;
  default?: string;
  enum?: string[];
  max_length?: number;
  precision?: number;
  scale?: number;
  order?: number;
}

export interface RelationDef {
  type: "belongsTo" | "hasMany" | "manyToMany";
  table: string;
  foreign_key: string;
  on_delete?: string;
  junction?: string;
  local_key?: string;
}

export interface IndexDef {
  columns: string[];
  unique?: boolean;
}

export interface ConfigField {
  key: string;
  type: "string" | "number" | "boolean" | "select" | "text";
  description?: string;
  required?: boolean;
  default?: unknown;
  options?: string[];
  placeholder?: string;
}

export interface MiddlewareDescriptor {
  name: string;
  description?: string;
  config_fields: ConfigField[];
}

export interface MiddlewareInstance {
  type: string;
  config: Record<string, unknown>;
}

export interface MiddlewareInfo {
  middleware: MiddlewareDescriptor[];
  presets: Record<string, string[]>;
  config: Record<string, Record<string, unknown>>;
  instances: Record<string, MiddlewareInstance>;
}
