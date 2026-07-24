# RealWorld Conformance — Findings

102 of 107 Hurl entries pass; 5 documented gaps (see `harness/known-failing.json`).

## How this was found

`examples/realworld` is a config-only Noda implementation of the
[RealWorld/Conduit](https://github.com/gothinkster/realworld) API spec — routes, workflows,
migrations, and tests, no application code. It's exercised against the vendored upstream Hurl
conformance suite (`harness/hurl/`, pinned at a fixed commit — see `harness/hurl/README.md`; the
files themselves are not edited) via `TestRealWorldConformance`
(`internal/testing/realworld/conformance_integration_test.go`, `-tags integration`), which boots
the real app with `cookbook.BootListen` against a real Postgres testcontainer, runs `hurl
--report-json`, and gates per **entry** (`"<file> :: entry <index> (line <line>)"`, both stable
across reruns) against the baseline in `harness/known-failing.json`: any newly-failing entry not
already in that baseline fails CI; any baseline entry that starts passing must be removed from the
baseline (closing the loop on regressions in both directions).

Current split: **102 passed, 5 known-gap, 0 unexpected** (13 files, 107 entries this run — the
exact entry count varies slightly run-to-run since a gapped entry gates how many downstream
entries in the same file execute, see the coverage caveat below).

## Coverage caveat

Hurl aborts a `.hurl` file's remaining entries after its **first failing entry** in that file
(empirically verified). Three of the five documented gaps below occur very early in their files:
`errors_articles.hurl` entry 1 (the file's very first request) and `errors_comments.hurl` entry 1
mean the entire rest of those two files — all their title/description/body blank-field checks,
duplicate-title handling, unknown-slug 404s, comment-blank-body/unknown-article/unknown-comment
checks — **never execute in this gate today**. `errors_auth.hurl` gets further (entries 1–9, the
register/login blank+duplicate-field checks, do run and pass) before hitting entry 10 (`GET /user`
no-auth), which masks entries 11–24 — all of the NIST-password-policy and null-vs-omitted-field
`update-user` checks. `favorites.hurl` entry 4 similarly masks the rest of that file, including the
whole unfavorite flow.

This masked logic is **not untested** — it's covered by the mock-based `noda test` suite
(`examples/realworld/tests/`, 30/30 passing) and, for the update-user null/omitted-field semantics
specifically, was additionally verified by hand against a real Postgres testcontainer during
development (register → `PUT bio=""` → `null`, `PUT bio=null` → clears rather than silently
keeping the old value, `PUT email=null` → 422, `PUT tagList=[]` → cleared, `PUT tagList=null` →
422, omit everything → all fields preserved). But it is not exercised by `TestRealWorldConformance`
itself, purely because of Hurl's per-file abort semantics colliding with the vendored suite's own
test ordering (no-auth checks placed early in each file). If issue #435 (below) is ever fixed,
expect the failing-set to *grow* before it can go green again — unmasking that content may surface
new, previously-untested failures.

## Documented gaps (`harness/known-failing.json`)

| Hurl key | Category | Root cause | Issue |
|---|---|---|---|
| `errors_articles.hurl :: entry 1 (line 2)` | noda-limitation | `auth.jwt` middleware's 401 body is fixed (`{"error":{"code":"UNAUTHORIZED",...}}`), not configurable; suite expects `{"errors":{"token":["is missing"]}}` | [#435](https://github.com/chimpanze/noda/issues/435) |
| `errors_auth.hurl :: entry 10 (line 116)` | noda-limitation | same `auth.jwt` fixed-body limitation, on `GET /user` with no token | [#435](https://github.com/chimpanze/noda/issues/435) |
| `errors_comments.hurl :: entry 1 (line 2)` | noda-limitation | same `auth.jwt` fixed-body limitation | [#435](https://github.com/chimpanze/noda/issues/435) |
| `errors_profiles.hurl :: entry 2 (line 8)` | noda-limitation | same `auth.jwt` fixed-body limitation | [#435](https://github.com/chimpanze/noda/issues/435) |
| `favorites.hurl :: entry 4 (line 45)` | noda-limitation | no in-workflow JWT decode/verify node — `get-article`'s route is deliberately unauthenticated, so a logged-in caller's identity can't be recovered and `article.favorited` is always `false` | [#434](https://github.com/chimpanze/noda/issues/434) |

All four `auth.jwt`-body entries assert the identical shape:
```
jsonpath "$.errors.token[0]" == "is missing"
  actual:   none
  expected: string <is missing>
```

## Broader Noda limitations found

These surfaced during the exercise but are outside the Hurl harness run itself (found while
building/fixing the app's workflows, per `.superpowers/sdd/task-8-report.md` and
`.superpowers/sdd/task-10-report.md`).

| Limitation | Where it surfaced | Issue |
|---|---|---|
| Engine join-classification misclassifies a diamond where a conditional's direct edge feeds a join while the other leg reaches it via an intermediate node (`internal/engine/compiler.go`'s `computeJoinTypes`/`hasCommonConditionalAncestor`) — misclassified as an AND-join, which hangs/fails the direct-edge path unconditionally | `workflows/update-user.json` (password vs. no-password-change diamond) and `workflows/update-article.json` (tag-replace presence-check diamond); worked around with a no-op node on the direct branch | [#433](https://github.com/chimpanze/noda/issues/433) |
| `db.update`'s `data` map resolution (`internal/plugin.ResolveMap`) is shallow — nested object values are copied verbatim, unresolved, silently writing raw `{{ ... }}` template text into the DB | `workflows/update-user.json`'s JSONB `metadata` column; worked around by pre-resolving the nested object in a `transform.set` node and referencing it as a single whole-`{{ }}` expression | [#438](https://github.com/chimpanze/noda/issues/438) |
| `db.update` exposes only a generic `error` output, no `exists`-style discriminator for unique-constraint violations (unlike `auth.create_user`) | `workflows/update-user.json`'s duplicate-email/username conflict path — the wired 422 also fires on unrelated DB errors | [#436](https://github.com/chimpanze/noda/issues/436) |
| No slugify/kebab-case expression function in `internal/expr` | `workflows/create-article.json` — slug generation had to be pushed into raw SQL instead of a workflow expression | [#437](https://github.com/chimpanze/noda/issues/437) |

## Fixed, not filed

An earlier gap — `auth.jwt` middleware rejecting a request with a `Bearer` scheme it didn't
recognize (`auth_scheme`) — was found and **fixed in this branch** (Task 0). No open issue is
needed for it.
