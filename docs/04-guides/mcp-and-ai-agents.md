# MCP & AI Agents

Noda ships an [MCP](https://modelcontextprotocol.io) server that exposes the runtime's
own knowledge — every node type, config schema, service schema, and expression function —
to AI coding agents. Because Noda projects are pure JSON config, an agent with these tools
can build and validate a working project without guessing at field names.

```bash
noda mcp
```

This starts a Model Context Protocol server over stdin/stdout. You do not normally run it
by hand; your agent's client launches it for you using the config below.

## Setup

### Claude Code

Project-scoped MCP servers are declared in a **`.mcp.json` file at the repository root**:

```json
{
  "mcpServers": {
    "noda": {
      "command": "noda",
      "args": ["mcp"]
    }
  }
}
```

To skip the per-server approval prompt for everyone working in the repo, add
`.claude/settings.json`:

```json
{
  "enableAllProjectMcpServers": true
}
```

> **`mcpServers` is not a valid key in `settings.json`.** Servers only load from `.mcp.json`
> (project scope) or `~/.claude.json` (user scope). `settings.json` has keys that *approve*
> `.mcp.json` servers — `enableAllProjectMcpServers`, `enabledMcpjsonServers` — but declaring
> a server there does nothing at all, silently.

Both files are written for you by `noda init` and are safe to commit.

### Other clients

Cursor, Windsurf, Zed, and the Claude Desktop app all take the same stdio server shape —
command `noda`, args `["mcp"]` — in their own config file. Consult your client's docs for
the file location. The only requirement is that `noda` is on the `PATH` of the process that
launches the server.

### Verifying the connection

Ask your agent to call `noda_list_nodes`. It should return 81 node types. If the tool isn't
available, the server didn't start — check that `noda version` works in your shell, and that
your config file is in the location your client actually reads.

> **Rebuild before you reconnect.** The MCP server is compiled into the `noda` binary, so a
> stale binary serves stale schemas. After changing plugins or node configs, rebuild `noda`
> and restart the MCP connection.

## Tools

Twelve tools, in two groups.

### Metadata tools (no project required)

These answer questions about Noda itself and work anywhere.

| Tool | Parameters | Purpose |
|---|---|---|
| `noda_list_nodes` | `category` (optional) | List all node types with descriptions, outputs, and service dependencies. Filter by prefix — `db`, `control`, `transform`, `auth`, `lk`, … |
| `noda_get_node_schema` | `node_type` (required) | JSON Schema for one node's config, e.g. `db.query`. Use after `noda_list_nodes`. |
| `noda_get_config_schema` | `config_type` (required) | JSON Schema for a config *file* type: `root`, `route`, `workflow`, `worker`, `schedule`, `connections`, `test`. |
| `noda_get_service_schema` | `plugin` (optional) | JSON Schema for a plugin's `services.*.config` block. Omit, or pass `all`, to get every service-bearing plugin at once. |
| `noda_list_functions` | — | Every function callable in an expression, both Noda built-ins and expr-lang built-ins. |
| `noda_validate_expression` | `expression` (required) | Check one expression's syntax and report the variables and functions it references. Expressions use `{{ }}` delimiters. |
| `noda_explain_workflow` | `workflow` (required), `input` (optional) | Statically analyze a workflow: execution order, data flow between nodes, expected output shapes. Does **not** execute it. |
| `noda_get_examples` | `pattern` (default `all`) | Example snippets for common patterns: `crud`, `auth`, `websocket`, `file-upload`, `scheduled-job`. |

### Project tools

These read or write a project on disk. **Every path must be absolute** — relative paths are
rejected rather than resolved against some ambiguous working directory.

| Tool | Parameters | Purpose |
|---|---|---|
| `noda_validate_config` | `config_dir` (required) | Validate a whole project: schema errors, missing references, cross-file issues. |
| `noda_scaffold_project` | `path` (required) | Create a new project with the standard layout and a generated `JWT_SECRET`. Refuses to overwrite existing files. |
| `noda_read_project_file` | `config_dir`, `path` (both required) | Read one config file. `path` is relative to the project, e.g. `workflows/hello.json`. |
| `noda_list_project_files` | `config_dir` (required) | List a project's config files, categorized by type. |

## Resources

The server also exposes documentation as MCP resources, so an agent can pull a full guide
into context instead of inferring from schemas alone:

| URI | Contents |
|---|---|
| `noda://docs/quick-start` | Getting started in five minutes |
| `noda://docs/expressions` | Expression syntax, context variables, built-ins |
| `noda://docs/expression-cookbook` | Expression recipes and type-coercion rules |
| `noda://docs/data-flow` | Trigger input, node outputs, aliases, data threading |
| `noda://docs/services` | Wiring infrastructure services to nodes |
| `noda://docs/realtime` | WebSocket/SSE channels and lifecycle hooks |
| `noda://docs/workflow-patterns` | Error handling, parallelism, caching, sub-workflows |
| `noda://docs/authentication` | JWT, OIDC, and Casbin setup |
| `noda://docs/testing` | Writing tests, mocking services, debugging |
| `noda://docs/migrations` | SQL migration format and the `noda migrate` CLI |

Two templated resources resolve dynamically:

- `noda://docs/nodes/{type}` — the reference page for one node, e.g. `noda://docs/nodes/db.query`
- `noda://schemas/{type}` — the raw JSON Schema for a config type

## A working loop

The tools are most effective in this order:

1. **`noda_get_config_schema("root")`** — establish the shape of `noda.json` first.
2. **`noda_get_service_schema()`** — get every plugin's service config at once, so services
   are wired with real field names rather than plausible-looking ones.
3. **`noda_list_nodes(category)`** then **`noda_get_node_schema(node_type)`** — pick nodes,
   then read each one's schema before configuring it.
4. **`noda_validate_expression`** — check non-trivial expressions individually. Cheaper than
   discovering the mistake through a whole-project validate.
5. **`noda_explain_workflow`** — confirm the graph does what you intended before writing it.
6. **`noda_validate_config`** — validate the project as a whole after every edit.
7. **`noda test`** — see the limitation below.

## Limitations worth knowing

**Validation does not run tests.** `noda_validate_config` checks that test files are
*structurally* valid; it never executes a workflow or compares output against a test's
`expect` block. **No MCP tool runs tests.** After editing a workflow through the MCP surface,
a passing `noda_validate_config` does not mean the tests still pass — run `noda test` in a
shell. This matters most for the scaffolded `tests/hello.test.json`, which asserts the output
of the *original* scaffolded workflow and goes stale the moment you change it.

**There are no write tools beyond scaffolding.** The server reads and validates; it has no
tool that edits a config file. Agents edit files with their own filesystem tools and then call
`noda_validate_config`. This is deliberate — it keeps edits visible in your normal diff review.

**Schemas come from the binary, not your source tree.** See the rebuild note above.

## See also

- [Testing & Debugging](testing-and-debugging.md) — `noda test`, mocking, dev mode
- [Service Wiring](../01-getting-started/services.md) — every plugin's service config
- [Node Reference](../03-nodes/_index.md) — all 81 node types
