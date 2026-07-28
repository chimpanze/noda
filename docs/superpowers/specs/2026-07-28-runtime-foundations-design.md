# Runtime Foundations — design for a fresh config-driven API runtime

**Date:** 2026-07-28
**Status:** design, pre-implementation
**Scope:** the foundations of a new runtime, informed by Noda. Not a Noda migration.

---

## 1. Why this document exists

Noda works. It is feature-complete, well tested, and has shipped six use cases. It also
produces a steady stream of small defects that share one cause, and no amount of careful
review has reduced the rate — because review finds instances and the cause is structural.

This document defines the language, the data flow, and the interfaces for a fresh start.
It is written to be implementable from scratch without reading Noda's source.

### 1.1 The measured problem

A census of Noda at commit `737ecb3`:

| Concept | Independent implementations |
|---|---|
| What variables exist in an expression | **6** — engine, HTTP edge, editor Go API, editor TypeScript, MCP mock, prose docs. They disagree: the editor omits `secrets` and offers `input.query`, which usually does not exist |
| What a node config schema permits | **4 validators** — a real JSON Schema library for config files, the same library for service configs, a 384-line hand-rolled walker for node configs, ajv in the editor — plus a fifth hand-written `if/else` in TypeScript choosing widgets |
| How a value becomes a string | **18** `case float64:` sites. Two use `strconv.FormatFloat`; the rest use `%v` or bespoke rules |
| How to walk the resolved config | **~20 files** |
| How a trigger builds workflow input | **5** — HTTP, worker, schedule, WebSocket, SSE. Worker and schedule share a helper; the other three do not |

Of the last 20 merged PRs, about 15 were substantive (the rest were dependency bumps and
housekeeping). Of those 15, **13** were one of: *N copies of one definition drifted*, *a gate
that had no teeth*, *a shared helper that handled some shapes and not others*, or *map
iteration order leaking into behavior*. Only two were ordinary feature work.

### 1.2 The cause

`pkg/api.NodeExecutor.Execute(ctx, nCtx, config map[string]any, services map[string]any)`
hands each node its **raw, unresolved config**. `grep Resolve internal/engine/` returns
nothing: the engine never evaluates node configuration. 61 plugin files each resolve their
own fields, choosing per field among 12 helpers, then coerce the result themselves.

**The language has no evaluation stage. Evaluation is delegated to the leaf.** With 103 node
types, there are 103 parsers.

Everything follows from that. Nobody can check an expression, because only the plugin knows
which fields contain expressions — so the validator must accept `{{ }}` anywhere, which is
why the hand-rolled walker exists. Coercion is per-leaf because it happens after resolution,
inside the plugin. The editor cannot know anything, because the knowledge exists only as
imperative Go inside 61 files, with no artifact to serve.

### 1.3 The rule this design is built on

> One definition of meaning, evaluated in one place, consumed everywhere else.

Not "a shared helper." Noda's validation pipeline was extracted into a shared helper four
times (#442 → #444 → #448 → #456) and drifted each time. A shared helper is a copy. The
fix that held was making the phase list *be* the boot path. This design generalizes that:
there is exactly one function from source to `Program`, and every surface calls it.

---

## 2. Architecture

```
┌─ BINDING ───────────────────────────────────────────────────────┐
│  trigger config       path/method │ topic/group │ cron │ channel │
│  middleware           auth, rate limit, CORS, CSRF               │
│  input mapping        trigger event    ──►  workflow input       │
│  outcome mapping      workflow outcome ──►  response form        │
└──────────────────────────────┬───────────────────────────────────┘
                               │ signature only
┌─ WORKFLOW ────────────────────▼──────────────────────────────────┐
│  input:  { … }                                                   │
│  output: created:{…} conflict:{…} invalid:{…} failed:Error       │
│  graph:  nodes, edges, scopes                                    │
│  knows nothing about HTTP, Redis, cron, or WebSockets            │
└──────────────────────────────┬───────────────────────────────────┘
                               │
┌─ SCOPES ──────────────────────▼──────────────────────────────────┐
│  immutable, single-assignment, append-only                       │
│  every parameter evaluated by ONE engine accessor, lazily        │
└──────────────────────────────┬───────────────────────────────────┘
                               │
┌─ NODE ────────────────────────▼──────────────────────────────────┐
│  receives typed parameter accessor + services. Never raw config. │
└──────────────────────────────────────────────────────────────────┘
```

Four language layers, specified in sections 3–7:

1. **Value** — what a value is (§3)
2. **Expression & parameter** — what a name means and how a parameter is written (§4)
3. **Execution** — scopes and lifetimes (§5)
4. **Workflow & binding** — signatures and wiring (§6, §7)

---

## 3. The value model

Every value in the language is a member of one closed type. **Not `any`.**

```
Value = Null
      | Bool(bool)
      | Number(literal-preserving)
      | String(string)
      | Bytes([]byte)
      | List([]Value)
      | Map(ordered, string-keyed)
```

### 3.1 Number preserves its literal

`json.Unmarshal` into `any` makes every JSON number a `float64` at load time. The value
`12345678901234567890` is lossy *before validation runs*, and a header carrying it is later
emitted as `1.2345678901234567e+19` by `%v`. `Number` holds the source text and exposes
integer, float and decimal views. Nothing downstream can lose precision, because nothing
downstream re-derives the value from a float.

### 3.2 Map iterates deterministically

Ordering is a property of the type, not of remembering to sort. Go's randomized map
iteration produced three separate Noda defects — nondeterministic error text, a
nondeterministic cycle report, and nondeterministic **entry-node dispatch order**, which
decided which branch of a workflow started first. With an ordered map type, none of the
three is expressible.

### 3.3 One conversion table

This table is normative. It is implemented exactly once.

| Value | → string | → query param | → header | → SQL arg | → JSON |
|---|---|---|---|---|---|
| `Null` | error | omitted | omitted | `NULL` | `null` |
| `Bool` | `true`/`false` | `true`/`false` | `true`/`false` | bool | bool |
| `Number` | exact literal | exact literal | exact literal | numeric | exact literal |
| `String` | itself | percent-encoded | validated, no CR/LF | text | string |
| `Bytes` | error | base64 | base64 | bytea | base64 |
| `List` | error | repeated `k=v` | repeated header | array | array |
| `Map` | error | error | error | JSON | object |

Inbound parsing mirrors it: a repeated query parameter and a repeated header both produce a
`List`. In Noda these diverged inside one file, twenty lines apart — headers preserved
repeats, query parameters silently dropped all but one.

### 3.4 Types

```
Type = Any | Null | Bool | Number | String | Bytes
     | List<Type> | Map<Type> | Record{ field: Type, … }
     | Union(Type, …) | Ref(name)
```

**Typing is gradual.** `Any` is assignable to and from anything, and reading a field of
`Any` yields `Any` without error. This is not a loophole — it is required: an HTTP response
body or an untyped database row is genuinely unknown at compile time. Static checks apply
wherever a shape is declared and step aside where it is `Any`. Every check in §8 fires only
when the relevant shape is known.

### 3.5 A stream is not a value

`Value` deliberately has no `Stream` member. A stream is consumed, has a lifetime and
backpressure, and can fail midway — so carrying one in `Value` would break three things this
design rests on:

- **§5.1 rule 3** (immutable once bound) — a stream observed twice is not the same stream
- **§5.7** (capture is free) — capturing a scope holding a stream is unsound; the snapshot
  is not a snapshot
- **§3 generally** — a live stream cannot be compared, logged, traced, or nested in a `Map`

A stream is not a value but a *process*. Streaming is therefore modelled as an **effect**
(§6.5), never as a value or a return type.

---

## 4. Expressions and parameters

### 4.1 The expression rule

One rule, in the language, applied by one parser. Never sniffed at a leaf.

| Written | Is | Yields |
|---|---|---|
| `"{{ input.limit }}"` — the **entire** string | expression | the expression's own type |
| `"page {{ input.n }} of {{ input.total }}"` | template | `String`, always |
| `"limit"` | literal | `String` |
| `"{{ '{{' }}"` or `\{{` | escape | literal braces |

Noda conflates expression and template: `{{ }}` is always embedded-in-string, so a node
wanting a number must resolve, then guess. Separating them lets a parameter's declared type
be compared against an expression's inferred type at compile time.

### 4.2 Parameter declaration

A node declares each parameter once. The engine evaluates from this declaration, the editor
renders from it, the documentation generates from it.

```go
type Param struct {
    Name        string
    Type        Type
    Kind        Kind        // Literal | Expression | Either
    Required    bool
    Default     Value
    Description string
}
```

`Kind` is Node-RED's `{value, type}` idea moved into the *declaration*, so configuration
stays plain JSON rather than every value becoming a tagged pair.

- `Literal` — must be statically known. This is what makes cross-reference checking possible
  without evaluating anything, and what lets a node compute its **output shape** at compile
  time (§6.2).
- `Expression` — must be an expression.
- `Either` — a literal of the declared type, or an expression that evaluates to it.

**Open question (§10.2):** whether `Either` should be the default.

### 4.3 One accessor, engine-owned and lazy

A node never receives raw configuration and never chooses a resolution helper.

```go
func (n *httpGet) Execute(ctx context.Context, run api.Run) (api.Result, error) {
    url, err := run.Params().String("url")   // evaluated now, against run's scope
    q,   err := run.Params().Map("query")
    …
}
```

Evaluation is **lazy and scope-bound**, which is what makes dynamic nodes work: a node
inside a for-each body receives an accessor bound to the *iteration* scope. A node that
genuinely needs extra bindings derives one:

```go
p := run.Params().With(map[string]Value{"row": row})
```

That is Noda's `ResolveWithVars` promoted from a node-private trick to a declared language
operation. **The node keeps control of when and in what scope. It never controls how.**

Errors from `Params` accessors mean *evaluation failed at runtime* (for example, a field
access on dynamic `Any` data that turned out absent). A type mismatch is not among them —
that is a compile error, or an engine bug.

---

## 5. Execution model — lexical scopes

### 5.1 Definition

A **scope** is an immutable set of bindings plus a parent pointer. Name resolution walks the
chain outward. Three rules:

1. **Single assignment.** A name is bound exactly once in a scope.
2. **Append-only.** Bindings are added as execution proceeds; none is removed or rewritten.
3. **Immutable once bound.** A bound value cannot change.

Together these make a scope *monotonic*: a reader may observe a binding as absent-then-present,
never as one value and later another.

### 5.2 The scope chain

```
root scope { secrets }
 │
 ├── trigger scope           ← the caller's frame; shape declared per trigger kind (§7.2)
 │     └── [message scope]   ← connection kinds only: one per inbound frame
 │
 │   input mapping evaluated HERE, against the trigger scope
 │   ══════════════ checked on both sides ══════════════════►
 │
 └── workflow scope { input }        ← parent is ROOT, not the trigger
      └── iteration scope { item, index }   ← one per for-each iteration
```

**The workflow scope's parent is the root scope, not the trigger scope.** A workflow is a
function; the trigger is a caller. Only declared input crosses the boundary. This is what
makes one workflow usable from an HTTP route, a worker and a schedule alike.

The input mapping is evaluated against the **innermost trigger-side scope** — the message
scope for connection kinds, the trigger scope for every other kind. Because scopes nest,
that mapping can still read connection-level bindings through the chain.

A long-lived connection is a genuine *parent* of each message scope. Noda flattens
`connection_id`, `channel`, `endpoint` and `user_id` into every message's input — a closure
expressed by copying.

### 5.3 Node outputs bind into the enclosing scope

A node's result binds under its node id. **A node executes at most once per scope**;
repetition happens by opening child scopes, never by re-running in place. So node bindings
are single-assignment by construction.

### 5.4 Definite assignment

The compiler knows the graph, so for any node it computes which other nodes are
**guaranteed** to have completed (on every path from entry) versus **possible** (on some
path). Reading a possible binding requires a guard.

| Expression at node N | Noda today | Here |
|---|---|---|
| `nodes.typo.x` | runtime `cannot fetch x from <nil>` | compile error — no such binding |
| `nodes.fetch.wrong_field` | runtime nil | compile error — output shape is declared |
| `nodes.a.x`, `a` on the other leg of an if/else | runtime nil, silent | compile error — `a` is possible, not guaranteed |
| `nodes.a.x ?? 'default'` | works by accident | accepted — the guard legalizes it |

This is ordinary definite-assignment analysis. It is available **only** because scope
contents are known statically, and it converts an entire class of runtime nils into compile
errors. Per §3.4 it fires only where shapes are known.

### 5.5 Iteration

A for-each construct opens one child scope per iteration, binding `item` and `index`. Body
node outputs bind into the *iteration* scope, so they neither leak across iterations nor
collide. The construct's result binds into the parent as the collected `List`.

Noda's `control.loop` hand-rolls this: it reads `config["collection"]` with a raw type
assertion, asserts `max_items` as both `float64` **and** `int` in consecutive lines because
no stage owns the number type, injects `$item`/`$index` privately, and needs a
`SubWorkflowRunner` plus a recursion-depth counter to iterate. The only declaration that
`$item` exists is a sentence in a schema `description` string.

Here: `item` and `index` are declared in the language, the body is a scope rather than a
sub-workflow, and body expressions are checkable because the iteration scope's shape is known.

### 5.6 Concurrency is free

Parallel branches write disjoint names — their own node ids — so single assignment means two
writers can never contend. **No mutex is needed in the semantics.** Noda's
`ExecutionContextImpl.Resolve` takes a read lock and rebuilds the entire context map on
every expression evaluation; both the lock and the rebuild exist because the binding table
is mutable and shared.

### 5.7 Capture is free

A node spawning work that outlives its scope captures it by holding the pointer. Because
scopes are immutable, a snapshot costs nothing and can never observe a later write. This is
why rule 3 is not optional.

### 5.8 Ambient execution metadata

`trace_id`, `request_id` and timestamps are telemetry, reachable from the runtime context —
**not** scope bindings. A workflow that wants `client_ip` or `user_agent` as *data* declares
them as input like anything else. This keeps signatures honest.

---

## 6. Workflows

### 6.1 A workflow is a function with a declared signature

```
workflow create-article
  input:
    title:     String
    body:      String
    author_id: String
  output:
    created:  { article: Article }
    conflict: { field: String }
    invalid:  { errors: List<ValidationError> }
    failed:   Error                              ← the default error outcome
```

Both halves are mandatory. Declaring only inputs makes half a signature; the output half is
what stops a workflow from being HTTP-bound.

Consequences:

- `{{ input.* }}` becomes checkable. In Noda `input` is wholly dynamic.
- Input coercion happens once, at the call boundary, driven by the declared schema. Noda has
  an open issue asking for exactly this, which exists only because there is no schema to drive.
- A workflow is testable by supplying input alone. Nothing to construct.
- "Optional auth" stops being an engine concern: a workflow needing caller identity declares
  `user_id` as input, and each binding supplies it or fails to check.

### 6.2 Node outputs

Every node declares every output with a **shape**:

```
db.create
  success → { row: Row }
  exists  → { conflicting_field: String }
  error   → Error
```

**Exhaustive wiring.** Every declared output is wired to an edge or explicitly discarded —
the default for all outputs, not a special category for "outcome outputs." Noda's bug class
where an unwired output silently ends the path (the workflow reports success and the route
answers `202 Accepted` while nothing happened) becomes unrepresentable, and the
`OutcomeOutputsProvider` interface plus its audit test are unnecessary.

**Shapes may be derived from `Literal`-kind parameters at compile time.** `db.query` with
`table: "articles"` derives its row shape from the declared model. This is why `Literal`
exists as a parameter kind (§4.2).

### 6.3 Errors

An error is a declared output with a declared shape (`code`, `message`, `retryable`) — not an
exception, not a special case. To keep exhaustive wiring from becoming verbose, a workflow
**designates one of its declared outcomes as the default error outcome**, and any unwired
`error` output propagates there. One line per workflow instead of one edge per node;
explicit, and still exhaustive.

The designation is explicit, not name-based: `failed` in §6.1 is an ordinary outcome that
the workflow marks as the error default. Nothing is special about the identifier.

### 6.4 Definite return

Terminal nodes bind to outcomes. The mirror of §5.4 applies: **every path must reach an
outcome, and every declared outcome must be reachable on some path.** A workflow with a
dangling path is a compile error.

### 6.5 Emission — streaming as an effect

Two distinct things get called streaming. A **long-lived connection carrying many messages**
(WebSocket, SSE-as-subscription) is already covered by the `connection` trigger kind (§5.2,
§7.2) and needs nothing further. What this section addresses is **one request whose response
body is produced incrementally** — chunked HTTP, an NDJSON export, a file download, an LLM
token proxy.

A workflow that streams declares an **emission shape** alongside its signature:

```
workflow export-articles
  input:  { author_id: String }
  emits:  { article: Article }
  output:
    done:      { count: Number }
    not_found: {}
    failed:    Error
```

A node declares in its descriptor that it emits, and with what shape. `Run` gains one method
(§9.1):

```go
Emit(Value) error
```

**Emission is an effect, like `db.create`.** It binds nothing into scope, so §3 and §5 are
untouched — which was the test any answer here had to pass. It composes with everything
already specified: a `http.stream` node emitting upstream chunks, a database cursor emitting
rows, a for-each construct (§5.5) emitting per iteration, with transformation in between.

One emission shape per workflow. Heterogeneous emissions use a `Union` shape. Multiple named
channels can be added later if a real case demands it; they cannot be removed.

**Backpressure.** `Emit` blocks, and returns an error on consumer disconnect or context
cancellation, so a slow consumer throttles the workflow rather than accumulating unbounded
buffer. The binding declares buffer size and overflow policy — block, drop-oldest, or fail
(§7.4).

**Testability.** A streaming workflow is tested by asserting the *sequence of emissions* plus
the terminal outcome. Noda's `sse.send` side effects are invisible to its test runner;
emissions are not.

---

## 7. The binding layer

### 7.1 What a binding is

The outer layer that connects a trigger to a workflow. **All trigger-specific knowledge lives
here** — status codes, dispositions, cron expressions, channel names, middleware.

```
route POST /api/articles → create-article
  middleware: [auth.session, ratelimit]
  input:
    title:     {{ request.body.title }}
    body:      {{ request.body.body }}
    author_id: {{ auth.sub }}
  output:
    created:  { status: 201, body: {{ out.article }} }
    conflict: { status: 409, body: { error: {{ out.field }} } }
    invalid:  { status: 422, body: {{ out.errors }} }
    failed:   { status: 500 }
```

One workflow, N bindings: the same `create-article` bound to an HTTP route, a worker topic
and a test harness — three independently checked wirings, one implementation.

Noda already has the *input* half of this in `routes/*.json`. The binding layer is that idea
completed, with the output half brought out of the workflow, where it was smuggled in as
`response.*` nodes.

### 7.2 A trigger kind is a declared contract

Each trigger kind declares two shapes, plus how it frames emissions (§6.5):

| Kind | Event shape (inbound) | Response shape (outbound) |
|---|---|---|
| `http` | `{ request: {method, path, query, params, headers, body}, auth? }` | `{ status, headers, body }` |
| `worker` | `{ message: {id, data, attempt}, topic, group }` | `ack \| retry \| dead_letter` |
| `schedule` | `{ schedule: {id, fired_at} }` | `∅` |
| `connection` | `{ connection: {id, channel, endpoint, user}, message }` | outbound message / broadcast |
| `wasm` | `{ module, tick }` | guest-defined |

And how emissions are framed, when the kind accepts them:

| Kind | An emission becomes |
|---|---|
| `http` | a chunk — NDJSON, SSE frame, or raw bytes, per the binding's declared framing |
| `connection` | a message to **this** connection |
| `worker` | one publish per emission to a declared topic — fan-out |
| `schedule` | discarded, or logged |
| `wasm` | guest-defined |

So one `export-articles` workflow can feed an HTTP NDJSON endpoint *and* a Redis topic,
differing only by binding. That is the binding layer paying off a second time.

Adding a trigger kind means declaring these shapes and writing a driver. **This is the
structural answer to Noda's five hand-rolled input builders**: there is one place a trigger
kind can be defined, and it is data.

The worker's response shape makes §5's scope-exit-as-disposition explicit and declared,
rather than inferred from success-versus-failure. One message is one scope; Noda's
`XReadGroup{Count: 1}` — chosen so each message has an independent ack/retry/dead-letter
fate — becomes the model rather than an implementation detail.

### 7.3 What is checked at a binding

- **Inbound:** each mapping expression against the event shape; the resulting value against
  the workflow's input schema.
- **Outbound:** exhaustiveness over the workflow's declared outcomes; each response value
  against the trigger's response shape (an HTTP `status` must be an integer; headers must be
  string-valued).

A route that fails to handle a declared outcome does not compile.

### 7.4 Streaming responses, and the head-commit rule

A binding for a workflow that declares `emits` (§6.5) states **two** mappings, because an
HTTP status must be sent before the body while the outcome is known only at the end:

```
route GET /api/articles/export → export-articles
  output:                          ← used if the workflow terminates BEFORE any emission
    not_found: { status: 404 }
    failed:    { status: 500 }
  stream:                          ← used once the first emission has occurred
    head:   { status: 200, content_type: "application/x-ndjson", framing: ndjson }
    buffer: { size: 256, overflow: block }        ← block | drop_oldest | fail
    on:
      done:   close
      failed: { event: "error", body: {{ out.code }} }
```

**The head is committed at the first emission.** Before it, any outcome maps normally — a
real `404`. After it, `200` has already been sent, so a failure can only be rendered *into*
the stream. Both mappings are checked for exhaustiveness over the workflow's declared
outcomes.

This is how HTTP actually behaves. Stating it in configuration is better than discovering it
at runtime, and it is why streaming is not simply another outcome shape.

### 7.5 Middleware placement stops being a judgment call

Auth, rate limiting, CORS and CSRF are trigger-layer concerns, therefore binding config by
definition. This also removes Noda's cross-reference blind spot, where the only middleware
form the route schema permits is the form the cross-reference pass skips — middleware
references become part of a checked artifact rather than an array a pass happens to ignore.

### 7.6 Emit is to the caller; publish is to someone else

A mid-workflow effect that targets a *different* consumer — broadcasting to a channel,
publishing to a topic other than the binding's — stays an ordinary node, like `db.create`.
Emission (§6.5) means "to my caller," and the binding decides how that is framed.

Noda conflates these: `sse.send` and `ws.send` serve both "reply to this connection" and
"push to that one." Separating them is what lets emissions be typed, checked and tested,
while genuine fan-out remains an effect.

Only *terminating* results are outcomes. Neither emissions nor effects are.

---

## 8. The compile pipeline

**There is exactly one function from source to `Program`, and every surface calls it.**

```
Source (files or bytes)
  │
  ├─ 1. Parse     JSON → Document tree. Every node carries Position(file, line, col, path).
  │                Numbers preserved as literal text.
  ├─ 2. Assemble  overlays merged; $env, $var, $ref resolved. Still a Document.
  ├─ 3. Bind      Document → Program AST. Trigger kinds, bindings, workflows, node
  │                instances. Parameters parsed into literal / template / expression (§4.1).
  ├─ 4. Check     the checks below.
  └─ 5. Program   immutable artifact.
```

`Program` is the single artifact. Every consumer reads it; nobody re-reads source.

**Validation is not a mode.** Checking is steps 1–4. Running is steps 1–5 plus execution.
This is the generalization of the one Noda fix that held — making the phase list *be* the
boot path — extended from the validation sequence to the whole pipeline.

### 8.1 The checks

Each check names the Noda defect class it structurally eliminates.

| # | Check | Eliminates |
|---|---|---|
| 1 | Document conforms to its schema (one JSON Schema library, no hand-rolled walker) | 4 validators disagreeing |
| 2 | Every parameter's expression type matches its declared type | expressions unverifiable at any stage |
| 3 | `Literal`-kind parameters are statically known | cross-reference checks that need evaluation |
| 4 | Definite assignment for every `nodes.*` read (§5.4) | runtime `cannot fetch x from <nil>` |
| 5 | Every node output wired or explicitly discarded (§6.2) | unwired output silently ends the path |
| 6 | Definite return: every path reaches an outcome (§6.4) | dangling path answering `202` |
| 7 | Binding input mapping type-checks both sides (§7.3) | typo-prone mappings with no validation |
| 8 | Binding output mapping is exhaustive over outcomes (§7.3) | unhandled outcome at runtime |
| 9 | Cross-references resolve (services, workflows, models, middleware instances) | reference blind spots |
| 10 | Graph is acyclic; joins and exclusivity groups are well-formed | cycle detected only at boot |
| 11 | A node that emits appears only in a workflow declaring `emits`, with a matching shape (§6.5) | untyped, untestable `sse.send` effects |
| 12 | A binding for an emitting workflow declares `stream`, and both its mappings are exhaustive (§7.4) | status committed before the outcome is known |

Checks 4, 5, 6, 8, 10 and 12 are graph- or binding-shaped. Noda validates node configs against schemas
without ever compiling the graph, which is why a config could pass validation and fail to boot.

### 8.2 Diagnostics

- Every diagnostic carries a **stable code**, severity, message, primary position, related
  positions, and an optional fix hint.
- Emission order is deterministic: sorted by `(file, line, column, code)`. Determinism comes
  from §3.2's ordered `Map` plus sorted emission, not from vigilance.
- One diagnostic list, several renderers (CLI text, JSON for the editor and MCP). A renderer
  never re-derives a diagnostic.
- Positions flow from step 1 into every diagnostic, so the editor gets precise markers
  instead of attribution bolted on afterward.

---

## 9. Interfaces

### 9.1 The plugin contract

```go
package api

// Node is the contract a node type implements.
type Node interface {
    Descriptor() Descriptor
    Execute(ctx context.Context, run Run) (Result, error)
}

type Descriptor struct {
    Type        string        // "db.create"
    Description string
    Params      []Param       // §4.2
    Outputs     []Output      // §6.2
    Services    []ServiceDep
}

type Output struct {
    Name        string
    Shape       Type
    Description string
}

// Run is everything a node receives. It never sees raw config.
type Run interface {
    Params() Params
    Service(slot string) (any, error)
    Log() *slog.Logger
    Tracer() Tracer

    // Emit streams one value to the caller (§6.5). Valid only in a workflow
    // declaring `emits`, which check 11 enforces at compile time. Blocks for
    // backpressure; returns an error on consumer disconnect or cancellation.
    Emit(Value) error
}

// Params is the single evaluation entry point. Lazy, bound to the current scope.
type Params interface {
    String(name string) (string, error)
    Number(name string) (Number, error)
    Bool(name string) (bool, error)
    Bytes(name string) ([]byte, error)
    List(name string) (List, error)
    Map(name string) (Map, error)
    Value(name string) (Value, error)
    Has(name string) bool
    With(bindings map[string]Value) Params   // derived accessor (§4.3)
}

type Result struct {
    Output string   // must name a declared output
    Value  Value
}
```

Compare Noda's `Execute(ctx, nCtx, config map[string]any, services map[string]any)`. The
single change of handing the node a `Params` instead of a `map[string]any` is what collapses
103 parsers into one.

### 9.2 Surfaces

| Surface | Consumes | Must never |
|---|---|---|
| `run` (boot) | `Program` | re-read source |
| `check` (validate) | steps 1–4 | keep its own phase list |
| `test` | `Program` | keep its own validation |
| editor | `Program`, over HTTP | validate locally |
| MCP | `Program` | keep its own validator |
| docs generator | `Program` descriptors | hand-write schemas |
| OpenAPI | `Program` bindings + signatures | walk the config separately |

**The editor holds zero semantic logic.** No ajv, no hardcoded variable list, no schema-shape
widget chain. It asks the server for validation, name scopes and field presentation. It is
already a dev-mode client of that server, so it has no offline story to protect.

---

## 10. Open questions

These are unresolved. They are recorded rather than silently decided.

> **Settled 2026-07-28 — streaming.** Previously listed here as the question that had to be
> answered before §6 could be built. Resolved as an *effect*, not an outcome shape: a stream
> cannot be a `Value` without breaking scope immutability and capture (§3.5), so streaming
> is `emits` on the signature plus `Run.Emit` (§6.5), with the head-commit rule at the
> binding (§7.4). `Value`, scopes, capture and the outcome union are unchanged.

### 10.1 Should `Either` be the default parameter kind?

Allowing a parameter to be literal-or-expression is ergonomic, and it is also the seam where
a weaker rule could creep back. The alternative — requiring each node to state the kind
explicitly — is more verbose but leaves no default to erode.

### 10.2 Does `Any` need a shape-assertion operator?

Gradual typing (§3.4) means dynamic data flows as `Any`. A way to assert a shape and get
static checking downstream (`as Article`, checked at runtime, typed thereafter) would recover
checking after an HTTP call or an untyped query. Not required for a first version.

### 10.3 Scope capture and memory retention

§5.7 makes capture free, but a captured scope keeps every binding alive. Long-lived deferred
work could retain arbitrarily large values. Needs either an eviction rule or an explicit
narrowing capture.

### 10.4 How model definitions feed output shapes

§6.2 sketches the mechanism — a node derives its output shape from `Literal`-kind parameters
plus declared models — but does not specify the interface a node implements to do it.

---

## 11. What is kept, and what is dropped

### Kept from Noda — these were right

- `pkg/api` as the plugin contract boundary
- Named multi-output edges, joins, exclusivity groups. Better than a single implicit input,
  and the main reason a Node-RED-style envelope model was rejected
- JSON configuration with overlays, `$env`, `$var`, `$ref`
- The plugin / service / node registry model
- Services as declared slots with declared config schemas
- The dev-mode editor as a server client
- Wasm guests via Extism
- The MCP surface

### Dropped

| Dropped | Replaced by |
|---|---|
| `map[string]any` as the value type | closed `Value` (§3) |
| Per-plugin resolution — 12 helpers across 61 files | one `Params` accessor (§4.3) |
| `response.json` / `response.redirect` / `response.error` nodes | binding outcome mapping (§7.1) |
| `workflow.output` / `SetWorkflowOutput` | terminal binding to a declared outcome (§6.4) |
| `OutcomeOutputsProvider` + audit test | exhaustive wiring (§6.2) |
| Sub-workflow-based loops with a depth counter | iteration scopes (§5.5) |
| Hand-rolled config-schema walker + vocabulary check | one JSON Schema library (§8.1) |
| Editor-side validation and widget mapping | server-served `Program` (§9.2) |
| Five hand-rolled trigger input builders | declared trigger kinds (§7.2) |
| `sse.send` / `ws.send` conflating "reply to my caller" with "push to another" | `Run.Emit` vs. publish nodes (§7.6) |

---

## 12. Non-goals

- **Not a general-purpose programming language.** Expressions stay expressions.
- **Not a data-pipeline engine.** No item model: a node is not run once per item, and no
  node signature carries a list. All five trigger types are one invocation per event; the one
  place batching could pay — the worker — deliberately reads one message at a time so each
  has an independent ack/retry/dead-letter fate. Emission (§6.5) is not a retreat from this:
  it streams *out* of one invocation as an effect, and never makes a node run more than once.
- **Not backwards compatible with Noda configuration.** This is a fresh start.

---

## 13. Implementation order

Each stage is usable before the next begins.

1. **`Value` and `Type`** (§3) — including the conversion table and its tests. Everything
   depends on this and nothing depends on the rest.
2. **Expression parser and type checker** (§4.1) — literal / template / expression, inference
   against a declared scope shape.
3. **Scopes** (§5) — the scope chain, single assignment, definite assignment.
4. **`Program` and the pipeline** (§8) — parse → assemble → bind → check, with positions and
   diagnostics.
5. **Node contract and `Params`** (§9.1) — then two or three real nodes end to end.
6. **Workflow signatures and outcomes** (§6.1–6.4).
7. **Binding layer and the `http` trigger kind** (§7.1–7.3, §7.5).
8. **Emission** (§6.5) and streaming bindings (§7.4) — after the non-streaming path works
   end to end, since emission is additive to the signature rather than a change to it.
9. **Remaining trigger kinds**, one at a time, each as declared shapes plus a driver.
10. **Surfaces** (§9.2) — `check`, `test`, editor API, MCP, all reading `Program`.

Node catalogue breadth is deliberately last. Noda's 81 node types were never the hard part;
the hard part was that each of them was a parser.
