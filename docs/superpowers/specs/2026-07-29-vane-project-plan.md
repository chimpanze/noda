# vane — project plan

**Date:** 2026-07-29
**Status:** plan
**Scope:** the whole project. Sits above `2026-07-28-runtime-foundations-design.md`
(cited below as **RF §n**), which remains normative for the language core: values,
expressions, scopes, workflow signatures, bindings, and the compile pipeline.

This document does three things RF deliberately does not:

1. **Decides RF's open questions** (§2). Where this document decides a question RF
   records as open, this document wins, and the decision date is noted.
2. **Specifies the subsystems RF leaves out** (§3–§6): the error model, services and
   plugins, middleware, the config surface, the node catalogue, the Wasm boundary,
   the testing ladder, and the tool surfaces.
3. **Sets the roadmap and its acceptance gates** (§8), extending RF §13.

Every subsystem section names the Noda defect class it is designed against. That is
the discipline RF established and the reason this plan exists: Noda's defects were
structural, so the successor's structure is chosen defect-first.

---

## 1. Product principles

RF §1.3 gives the implementation rule: *one definition of meaning, evaluated in one
place, consumed everywhere else.* This plan adds three project-level rules.

### 1.1 Strict inside, forgiving outside

Strictness lives at compile time, where a diagnostic can carry a position and a fix
hint. The user-facing consequence of a strict core is *better* ergonomics, not
worse: `vane check` rejecting a typo with `bindings/articles.json:14:7 — workflow
'create-articel' does not exist (did you mean 'create-article'?)` is friendlier than
Noda's `valid: true` followed by a boot failure or a silent `202`. Anywhere this
plan chooses between author convenience and a checkable rule, it takes the rule and
spends the effort on the diagnostic instead.

### 1.2 An artifact that can drift is generated or compiled — no third option

Noda ended up with **five bespoke CI gates** (doc snippets, cookbook coverage,
TinyGo guests, auth fixtures, scaffold alignment) because docs, examples, fixtures,
scaffold output and Wasm guests were all hand-maintained copies of runtime truth.
The census is unambiguous: every category of artifact eventually drifted, and every
fix was another gate.

vane's rule: every artifact is either **generated from `Program`** (node reference,
config reference, OpenAPI, editor types, PDK) or **compiled by the one pipeline in
CI** (examples, doc snippets, scaffold output, test fixtures). Nothing is verified
by a bespoke checker, because a bespoke checker is a copy of the pipeline.

### 1.3 Fewer things, finished

A subsystem ships when it has: a declared contract, its checks in the pipeline
(RF §8.1), generated documentation, and tests at the right rung of the ladder
(§6). Until then it does not ship. Noda's 81 nodes, 11 config kinds and 22 editor
endpoints were breadth ahead of depth; vane starts at roughly a third of that
surface (§5) and grows only behind the gates.

---

## 2. Decisions on RF §10's open questions

### 2.1 Parameter kind has no default (RF §10.1) — decided 2026-07-29

`Param.Kind` is **mandatory and explicit**. A descriptor whose parameter has the
zero `Kind` is rejected at registration — a startup panic in development, a compile
error for in-tree plugins via a registry test.

RF names `Either` "the seam where a weaker rule could creep back." A default is
exactly such a seam: it turns an omission into a semantic choice. Verbosity is
handled by construction helpers, not by defaulting:

```go
api.Lit("table", types.String, api.Required)          // Kind: Literal
api.Expr("filter", types.Map(types.Any))              // Kind: Expression
api.Either("limit", types.Number, api.Default(vint(50)))
```

The helpers make the explicit form one token longer than a default would be. That
is the whole cost, and it buys an unerodable rule.

### 2.2 `Any` gets a shape assertion (RF §10.2) — decided 2026-07-29, scheduled stage 11

One expression operator: `expr as TypeRef`, where `TypeRef` names a declared model
or type alias in the Program's type environment.

- **Compile time:** the expression's type downstream of `as` is the asserted type,
  so definite-assignment and field checks (RF §5.4) resume after an HTTP call or an
  untyped query.
- **Runtime:** the value is checked against the type (the existing
  `types.Check`). Failure is an evaluation error — it surfaces exactly like a
  failed `Params` accessor (RF §4.3): the node's `error` output fires with a
  structured `Error` (§3) whose `details` carry the path that mismatched.

Not in v1 of the expression checker: it lands in stage 11 (§8), after the
non-streaming path is end-to-end, because nothing in stages 1–10 depends on it and
gradual `Any` flow is sound without it (RF §3.4).

### 2.3 Capture is inferred, and narrowing is declared (RF §10.3) — decided 2026-07-29

RF §5.7 makes capture free but unbounded. The resolution uses what vane has and
Noda lacked: expressions are parsed once, by the engine, so **free variables of any
deferred body are statically known**. Noda's `EvictionTracker` scraped identifiers
out of strings with a hand lexer; vane computes the same set soundly from the AST.

Rules:

1. When a node detaches work (deferred effect, post-response continuation), the
   engine computes the deferred body's free-variable set and retains **only those
   bindings**. The rest of the scope is releasable immediately.
2. A binding read through dynamic `Any` access (`nodes[input.which]`) defeats
   inference for that expression; the engine then retains the named node map it is
   indexed from, and `vane check` emits an *info*-level diagnostic so wide
   retention is visible, not silent.
3. An explicit `capture: [a, b]` list on the detaching construct narrows further.
   Reading outside the declared list is then a compile error where inference can
   see it, and a runtime evaluation error where it cannot.

This also subsumes eviction inside a live workflow: the compiler already knows the
last reader of every binding, so scope memory is released on the same analysis.
One analysis, two consumers — retention and eviction — instead of Noda's
string-scraping tracker.

### 2.4 Models feed output shapes through one interface (RF §10.4) — decided 2026-07-29

Models compile into the Program's type environment as named `Record` types:
`model:articles` is a `Ref` target like any other. A node that derives its output
shape implements one optional interface, invoked during **Bind** (RF §8, step 3):

```go
// ShapeDeriver lets a node compute output shapes from Literal-kind
// parameters at compile time. Optional; nodes with static shapes
// declare them directly in Descriptor.Outputs.
type ShapeDeriver interface {
    DeriveOutputs(lits Literals, env TypeEnv) ([]Output, error)
}

type Literals interface {          // the Literal-kind params, already checked
    String(name string) (string, bool)
    Value(name string) (value.Value, bool)
}

type TypeEnv interface {           // read-only view of the Program's types
    Lookup(ref string) (types.Type, bool)
}
```

`db.query` with `table: "articles"` looks up `model:articles` and returns
`success → { rows: List<Ref("model:articles")> }`. An error from `DeriveOutputs`
is a diagnostic positioned at the literal parameter, not a runtime surprise. The
editor and docs generator see the derived shape because it is part of `Program` —
this deletes the last reason Noda's editor hardcoded per-node output derivers in
TypeScript (`configDerivedSchema.ts`).

---

## 3. The error model — one vocabulary, one shape

**Noda defect class:** four independent error-code vocabularies (`pkg/api/errors.go`
admits to the other three in its own doc comment), plus a fifth ad-hoc shape minted
in the engine for error-edge payloads, plus a test-runner assertion language that
leaked Go struct field names (`Body`, capital B) into user JSON.

vane has **one error shape**, expressible as a `Value` and declared as a `Record`
type in the root type environment:

```
Error = { code: String, message: String, retryable: Bool, details: Map<Any> }
```

- **Codes are a closed registry.** `internal/errcode` defines `type Code string`,
  a `Register(code, doc, httpStatus, workerDisposition)` call, and rejects
  duplicates. The registry is data, so the docs page, the HTTP status mapping, and
  the worker retry/dead-letter decision are all *lookups*, never switch statements.
  A code used anywhere without registration fails a registry test.
- **Every surface renders the same value.** A node's `error` output, a workflow's
  designated error outcome (RF §6.3), the HTTP error handler, the worker
  dead-letter envelope, and the Wasm error frame (§5.3) all carry `Error` and
  render it through the §1.2 conversion table. There is nothing else to drift.
- **Test assertions address it as data**, by path over the declared shape
  (`outcome.failed.code == "CONFLICT"`), checked at compile time against the
  workflow signature — the `Body`-vs-`body` class is a compile error (§6.2).

---

## 4. Services, plugins, and middleware

### 4.1 Split the plugin contract

**Noda defect class:** `Plugin` conflated node-provider and service-provider; 4 of
its 7 methods were dead stubs in all 12 core plugins, a `compositePlugin` shim
re-glued the halves, `CreateService` returned `any`, and a second "deferred
service" registration path was bolted on beside the first.

vane has two contracts and no shim:

```go
type NodeProvider interface {
    Prefix() string          // validated: ^[a-z][a-z0-9]*$
    Nodes() []Node
}

type ServiceProvider interface {
    Kind() string                    // "postgres", "redis-cache", …
    ConfigSchema() types.Type        // a Type, not a hand-written map
    New(ctx context.Context, cfg value.Map) (Service, error)
}

type Service interface {
    HealthCheck(ctx context.Context) error
    Shutdown(ctx context.Context) error
}
```

A package exporting both implements both; the registry keeps two tables. Service
configs are validated against `ConfigSchema()` by the one pipeline — Noda's
`security.casbin: {"type":"object"}` opacity is unrepresentable because a
`types.Type` cannot be silently empty of meaning in the same way (and check 1
validates it like every other document).

**One registration path.** Wasm runtimes and connection endpoints are services
constructed by `ServiceProvider`s like any other — Noda's `DeferredService`
side-channel does not exist.

### 4.2 Typed service slots

A node declares `ServiceDep{Slot, Kind}`. Binding (RF §8 step 3) checks the slot's
configured service is of the declared kind, so the mismatch is a compile
diagnostic. At runtime the node recovers the concrete interface through one
generic accessor:

```go
db, err := api.ServiceAs[*postgres.DB](run, "database")
```

`ServiceAs` failing is by construction an engine or registration bug, never a
config error — config errors were consumed at compile time.

### 4.3 No monolithic plugin list

**Noda defect class:** `plugins/all` pulled the cgo image plugin into everything,
which kept `internal/validate` out of the editor for months.

vane has no `plugins/all`. The scaffold emits an explicit plugin list in
`main.go`; heavyweight or cgo-backed providers (image processing, LiveKit) live in
separate Go modules, imported only by projects that use them. The core binary is
pure-Go and static.

### 4.4 Middleware is a declared contract

**Noda defect class:** middleware had no schema introspection (it isn't a node, so
every schema tool returned not-found), `security.casbin` was an opaque object, and
`worker.ResolveMiddleware` silently dropped unknown names — a bug with, in the
words of the design doc that found it, "no phase to live in."

vane middleware uses the **same `Param` declaration as nodes** (RF §4.2):

```go
type Middleware interface {
    Descriptor() MiddlewareDescriptor   // Type, Description, Params
    Wrap(next Handler, params Params) Handler
}
```

Consequences, all free: the editor renders middleware config from the same
presentation logic as node config; docs are generated from the same descriptors;
and an unknown middleware reference is caught by RF check 9 like any other
cross-reference — there is exactly one phase for it to live in, so it cannot be
homeless.

---

## 5. The user-facing surface

### 5.1 Config: seven file kinds, one discovery table

**Noda defect class:** 11 config kinds, each addition touching a hardcoded
6-field discovery struct plus five call sites; `connections` advertised in the
root schema but silently ignored by the loader; migrations living entirely outside
the config system.

vane cuts the kinds to **seven**, because RF's binding layer collapses four of
Noda's kinds into one:

| Kind | Directory | Replaces in Noda |
|---|---|---|
| root | `vane.json` + `vane.<env>.json` | `noda.json` + overlays |
| vars | `vars.json` | same |
| workflows | `workflows/*.json` | same |
| **bindings** | `bindings/*.json` | `routes/`, `workers/`, `schedules/`, `connections/` |
| models | `models/*.json` | `models/` + `schemas/` (folded together) |
| tests | `tests/*.json` | same |
| migrations | `migrations/*.sql` | same — but declared, discovered and listed by the pipeline |

A binding file's `kind` field (`http`, `worker`, `schedule`, `connection`, `wasm`)
selects the trigger contract (RF §7.2). One concept, one directory, one schema
with a discriminator — not four directories with four schemas that share nothing.

Discovery is **one table consumed by Assemble** (RF §8 step 2):

```go
type KindSpec struct {
    Name   string
    Glob   string
    Schema *jsonschema.Schema     // the only schema for this kind, embedded
    Bind   func(doc Document, p *ProgramBuilder) []Diagnostic
}
var Kinds = []KindSpec{ … }       // adding a kind is adding one entry
```

The root schema is *generated over* this table, so the schema cannot advertise a
kind the loader does not read — the Noda `connections`-in-root bug class is
unrepresentable.

### 5.2 The node catalogue: ~30 nodes, one grammar, one outcome lexicon

**Noda defect class:** 81 nodes with one camelCase stray (`db.findOne`), three
competing "already exists / bad input / missing" port conventions, an 8-of-81
partial enforcement interface, and a hardcoded static-fields table in the
framework because the property had no home on the node.

vane launches with a deliberately small catalogue — the RF closing line is the
budget: *"Noda's 81 node types were never the hard part."*

| Family | Nodes (initial) |
|---|---|
| `db` | `query`, `get`, `create`, `update`, `delete`, `transaction` |
| `cache` | `get`, `set`, `delete` |
| `http` | `request`, `stream` |
| `control` | `if`, `switch`, `for_each` (RF §5.5 construct) |
| `transform` | `set`, `map`, `pick`, `merge` |
| `auth` | `create_user`, `verify_credentials`, `create_session`, `require` |
| `publish` | `topic`, `channel` (RF §7.6 — effects to *someone else*) |
| `storage` | `read`, `write`, `delete`, `list` |
| `util` | `log`, `uuid`, `delay` |
| `workflow` | `run` |
| `email`, `wasm` | one node each |

Rules enforced at registration, not by review:

- **Naming law:** node type matches `^[a-z][a-z0-9]*\.[a-z][a-z0-9_]*$`. The
  registry rejects violations; there can be no second `findOne`.
- **Outcome lexicon:** output names come from a closed set — `success`, `error`,
  `exists`, `invalid`, `not_found`, `then`, `else`, `done` — with meanings
  documented once. A new name requires adding it to the lexicon (a reviewed,
  documented act), not just typing it in a descriptor. With declared shapes
  (RF §6.2) the names are less load-bearing than in Noda, but a shared lexicon is
  what keeps `exists` vs `conflict` vs `duplicate` from re-diverging.
- **Static-vs-expression lives on the parameter** (`Kind`, §2.1). The framework
  table (`staticFieldsByNodeType`) has nothing left to hold and does not exist.

Gone entirely, per RF §11: `response.*` (binding outcome mapping), `workflow.output`
(terminal outcome binding), `sse.send`/`ws.send` (split into `Emit` and
`publish.channel`), `control.loop`-as-sub-workflow (`control.for_each` iteration
scopes). The LiveKit family (18 nodes) moves out of tree per §4.3.

### 5.3 The Wasm boundary: one declaration, generated ends

**Noda defect class:** the host-operation vocabulary existed in four places
(service interfaces, host dispatcher switches, `operationsForPrefix`, PDK
wrappers); every wire struct was declared twice with dual codec tags; and the
0-offset "void success" sentinel silently converted a host-side write failure into
guest-visible success (issue #267).

vane declares each host operation **once, as data**:

```go
type HostOp struct {
    Name     string          // "cache.get", "system.set_timer"
    Request  types.Type
    Response types.Type
    Doc      string
}
var HostOps = []HostOp{ … }
```

From this table a generator (`go generate`) produces: the host-side dispatch
(unmarshal → check → call → marshal), the Go PDK stubs with typed signatures, the
guest-visible service manifest, and the Wasm host API documentation page. Adding
an operation is one table entry plus one host method; the four copies cannot
disagree because three of them are outputs.

Wire protocol decisions:

- **One codec: JSON**, chosen for debuggability. msgpack returns only if profiling
  demands it, and then through the same generator — never as parallel hand-written
  tags.
- **Explicit envelope, no sentinels.** Every host call returns a serialized
  `{ status: "ok" | "error", data?, error?: Error }` frame — the §3 `Error` shape.
  There is no magic offset; a host-side failure is always an inspectable frame.
- **State is explicitly single-instance**, as in Noda, but the limitation is
  *declared* (§7) and softened by two generated host ops — `state.save(bytes)` /
  `state.load() bytes` backed by the storage service — so persistence is one call
  on the author's chosen interval rather than a hand-rolled pattern.

### 5.4 Expressions: the user-visible contract

RF §4 fixes the semantics; two additions are user-surface:

- **Function signatures are declared once.** Noda's built-ins hand-checked arity
  in every body and stated each signature twice (once for the checker, once in
  code). vane registers functions with a typed signature the checker and the
  runtime both consume:

  ```go
  RegisterFunc("sha256", Sig(types.String).Returns(types.String), impl)
  ```

- **`$env`/`$var` stop wearing the expression costume.** In Noda, three grammars
  shared `{{ }}` delimiters, two of them regex-only. In vane, `$env()` and
  `$var()` are ordinary functions in the *one* expression language, restricted by
  the checker to the Assemble stage (a compile diagnostic if used where they
  cannot be resolved). One parser, one grammar, staged availability.

---

## 6. The testing ladder — every rung first-class

**Noda defect class:** `noda test` bypassed routing, auth, middleware and
transport entirely (it called `engine.Compile` + `ExecuteGraph` directly); all
five AI builders shipped projects that validated clean and failed `noda test`;
one validated clean and could not boot; the harness itself initially ran static
validation only and reported success.

The ladder, each rung consuming the same `Program` (RF §9.2):

| Rung | Command | What runs | What it catches |
|---|---|---|---|
| 1 | `vane check` | pipeline steps 1–4 | everything in RF §8.1 — including graph and binding checks Noda never ran |
| 2 | `vane test` | `Program` + **real trigger drivers in-process** | binding bugs: routing, input mapping, middleware, auth, outcome mapping |
| 3 | `vane test --live` | full boot (steps 1–5) against real services | service config, migrations, connection lifecycles |
| 4 | AI-usability harness | the Noda harness, ported | the gaps the other rungs didn't know to look for |

Decisions that make rung 2 honest where Noda's was not:

- **Tests exercise bindings, not bare workflows.** An HTTP test drives a real
  in-process listener through the actual middleware chain and outcome mapping; a
  worker test injects a message through the actual disposition logic. "It works in
  `vane test`" therefore means the endpoint works, not that a graph executed.
- **Assertions are paths over declared shapes**, checked at compile time:
  `expect: { outcome: created, response.status: 201, response.body.article.title: "Hi" }`.
  A path that does not exist in the declared shape is a rung-1 diagnostic. The
  `Body`-capital-B class — Go struct internals leaking into user JSON — cannot
  exist because assertions never address Go structs.
- **Emissions are assertable** (RF §6.5): a streaming test asserts the emission
  sequence plus the terminal outcome.
- **Boot parity is structural, not maintained.** Rung 3 *is* steps 1–5. There is
  no separate validation list to drift, which is the RF §8 rule; the four-times-
  recurring "validate says yes, boot says no" bug has no home.

**Rung 4 is the acceptance gate for the project itself.** Noda's AI-usability
harness (briefs → adversarial evaluator → runtime verification → e2e) is ported
early (§8, stage 14) and run against every release candidate. vane 1.0 ships when
a harness generation completes with **zero confirmed findings including the
runtime and e2e phases** — the standard Noda reached only after three
generations of retrofit.

---

## 7. Declared limitations

Noda repeatedly *discovered* its limitations in production or in the harness.
vane declares them up front, in generated docs, as contract:

- **Wasm module state is per-instance.** No transparent horizontal scaling of
  stateful modules; `state.save`/`state.load` (§5.3) is the offered mitigation.
- **Cross-instance connection delivery is fan-out** (Noda §19.5's decision,
  kept deliberately): O(instances) per message, no registry to corrupt.
- **Expressions are not a programming language** (RF §12). No user-defined
  functions in v1.
- **The catalogue is small on purpose** (§5.2). Breadth is the last stage, behind
  the gates, and some Noda families (LiveKit, image) return only as out-of-tree
  modules.

---

## 8. Roadmap

Stages 1–10 are RF §13, unchanged — stage 1 (Value and Type) is already planned in
`2026-07-28-value-and-type-layer.md` and under way. This section adds the gates
each stage must pass, and stages 11–16.

**Standing gates, from stage 1 onward** (the §1.2 rule made operational):

- Every test observed to fail before its implementation exists (the stage-1 plan's
  discipline, kept for all stages).
- Every example and doc snippet in the repo compiles through the one pipeline in
  CI. This gate exists from the *first* example, not retrofitted at milestone 25.
- Every diagnostic added carries a stable code, a position, and a test asserting
  its exact rendering.
- No exported `any` outside the boundaries the design names (`ToSQLArg`,
  `Service(slot)`).

| Stage | Delivers | Gate to close it |
|---|---|---|
| 1–10 | RF §13: value/type → expressions → scopes → pipeline → node contract → signatures → http binding → emission → remaining trigger kinds → surfaces | each stage's checks live in the pipeline; `check`/`test`/editor/MCP read `Program` only |
| 11 | `as` operator (§2.2); capture inference + `capture:` narrowing (§2.3) | definite-assignment resumes after `as`; retention test: deferred work holds only its free variables |
| 12 | Wasm boundary (§5.3): HostOp table, generator, PDK, `state.save/load` | generated dispatcher and PDK diff-clean in CI; envelope has no sentinel path |
| 13 | Auth service + middleware set (§4.4); errcode registry complete (§3) | middleware configurable only via declared `Param`s; every emitted code registered |
| 14 | Generators: scaffold, node/config/host-API reference, OpenAPI; **AI-usability harness ported** | scaffold output passes rungs 1–3 in CI; docs pages are build artifacts, not sources; first harness generation runs |
| 15 | Editor (thin client per RF §9.2): Program view, diagnostics with positions, scope-aware completion, param-driven forms, trace | editor TS types generated from Go; a grep gate proves the editor source contains no variable list, widget table, or validator |
| 16 | Catalogue to ~30 nodes (§5.2); examples for each RF trigger kind; migrations integrated into the pipeline | naming law and outcome lexicon enforced by registry tests; every node's doc page generated; harness rerun |

**1.0 criterion:** one full AI-usability harness generation — static, runtime,
and e2e phases — with zero confirmed findings, on a project scaffolded by
`vane init` and built only through the MCP surface. That is the measurable form
of "simpler to use": an agent given only the tool surface builds a working,
booting, tested project without tripping over a single contradiction between two
statements of the same fact.

**The census as a KPI.** RF §1.1's table is re-measured at each stage close. The
target column is `1` for every row: one statement of the expression scope, one
schema validator, one value-to-string conversion, one config walker, one trigger
input builder. Any measurement above 1 is a defect with a stage assigned, not a
note.

---

## 9. Relationship to Noda

- Noda stays as-is: complete, green, the reference corpus. Its harness, its
  examples and its defect record are inputs to vane's gates.
- Nothing in vane imports Noda. Configuration is not compatible (RF §12) and no
  migration tooling is planned until vane 1.0 exists.
- This document, like RF, is copied into the vane repository so it carries its
  own specification (`docs/design/project-plan.md`); the copy in the Noda tree is
  the design record.
