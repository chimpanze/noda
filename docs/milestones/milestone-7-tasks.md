# Milestone 7: Workflow Testing Framework — Task Breakdown

**Depends on:** Milestone 5 (core control nodes), Milestone 6 (transform + utility nodes)
**Result:** `noda test` runs workflow tests with mock nodes. Core nodes (control, transform, utility) execute normally while plugin nodes are replaced with configurable mocks. Test results report pass/fail with execution traces for failures.

---

## Task 7.1: Test File Loader

**Description:** Load and parse test definition files from `tests/` directory.

**Subtasks:**

- [ ] Create `internal/testing/loader.go`
- [ ] Implement `LoadTests(config *ResolvedConfig) ([]TestSuite, error)`:
  - Read test files already loaded during config phase
  - Parse each into a `TestSuite` struct
- [ ] Define `TestSuite` struct:
  ```
  TestSuite {
      ID       string
      Workflow string          // referenced workflow ID
      FilePath string
      Cases    []TestCase
  }
  ```
- [ ] Define `TestCase` struct:
  ```
  TestCase {
      Name    string
      Input   map[string]any         // $.input for the workflow
      Auth    *api.AuthData          // optional $.auth
      Mocks   map[string]MockConfig  // keyed by node ID
      Expect  TestExpectation
  }
  ```
- [ ] Define `MockConfig` struct:
  ```
  MockConfig {
      Output     any              // data to return on success
      OutputName string           // for workflow.run mocks: which output name to fire
      Error      *MockError       // if set, mock fails with this error
  }
  ```
- [ ] Define `TestExpectation` struct:
  ```
  TestExpectation {
      Status    string           // "success" or "error"
      Output    map[string]any   // field assertions on the final output (dot-path → expected value)
      ErrorNode string           // if status="error", which node should have failed
  }
  ```
- [ ] Validate: referenced workflow must exist, mocked node IDs must exist in the workflow

**Tests:**
- [ ] Load valid test file
- [ ] Parse test case with mocks, input, and expectations
- [ ] Validation catches non-existent workflow reference
- [ ] Validation catches non-existent node ID in mocks

**Acceptance criteria:** Test files are parsed into structured test suites with validation.

---

## Task 7.2: Mock Node Executor

**Description:** A mock node executor that replaces plugin nodes during testing.

**Subtasks:**

- [ ] Create `internal/testing/mock.go`
- [ ] Implement `MockExecutor` satisfying `api.NodeExecutor`:
  - Created from a `MockConfig`
  - `Outputs()` — returns `["success", "error"]` or, for `workflow.run` mocks, the configured output name + `"error"`
  - `Execute()`:
    - If `MockConfig.Error` is set → return the error (engine routes to `"error"` output)
    - If `MockConfig.OutputName` is set → return that output name with `MockConfig.Output` data
    - Otherwise → return `"success"` with `MockConfig.Output` data
- [ ] Mock preserves the original node's service deps and config schema (not checked during test execution, but preserved for reference)
- [ ] `MockConfig.Output` can be `nil` (mock returns nil data on success)

**Tests:**
- [ ] Mock success returns configured output data
- [ ] Mock error returns configured error
- [ ] Mock with OutputName fires the named output
- [ ] Mock with nil output returns nil on success
- [ ] Mock satisfies NodeExecutor interface

**Acceptance criteria:** Mock executor replaces any plugin node with configurable behavior.

---

## Task 7.3: Test Runner

**Description:** Execute test cases against workflows, replacing plugin nodes with mocks.

**Subtasks:**

- [ ] Create `internal/testing/runner.go`
- [ ] Implement `RunTestSuite(suite TestSuite, config *ResolvedConfig, ...) []TestResult`:
  - For each test case:
    1. Load the referenced workflow
    2. Replace all nodes listed in `Mocks` with `MockExecutor` instances
    3. Core nodes (control, transform, util, response) execute normally — NOT mocked
    4. Create execution context with `$.input` from test case, `$.auth` from test case
    5. Execute the workflow through the engine
    6. Compare results against expectations
    7. Record pass/fail with details
- [ ] Define `TestResult`:
  ```
  TestResult {
      CaseName string
      Passed   bool
      Expected TestExpectation
      Actual   TestActualResult
      Error    string           // failure reason if not passed
      Trace    []TraceEvent     // execution trace for debugging
      Duration time.Duration
  }
  ```
- [ ] `TestActualResult` captures: final status, output data, error node (if failed)
- [ ] Unmocked plugin nodes (not in mocks map) → fail with clear message: "Node 'X' has no mock. Add a mock or use a core node."

**Tests:**
- [ ] Test case passes when expectations match
- [ ] Test case fails when output doesn't match expected
- [ ] Test case with error expectation passes when workflow errors at expected node
- [ ] Unmocked plugin node produces clear error
- [ ] Core nodes execute normally (not mocked)
- [ ] Auth data passed through to execution context
- [ ] Trace events captured for each test case

**Acceptance criteria:** Test runner executes workflows with mocks and reports pass/fail accurately.

---

## Task 7.4: Expectation Matching

**Description:** Compare actual execution results against expected values.

**Subtasks:**

- [ ] Create `internal/testing/match.go`
- [ ] Implement `MatchExpectation(expected TestExpectation, actual TestActualResult) (bool, string)`:
  - Match status: expected `"success"` vs actual status
  - Match output fields: for each `"dotted.path": expectedValue` in `expected.Output`, extract the path from actual output and compare
  - Match error node: if `expected.ErrorNode` is set, verify the workflow failed at that specific node
- [ ] Dot-path extraction: `"response.body.email"` → navigate into nested maps
- [ ] Comparison: deep equality for maps/arrays, value equality for primitives
- [ ] Partial matching: only fields listed in `expected.Output` are checked — extra fields in actual output are ignored
- [ ] Return `(true, "")` on match, `(false, "explanation")` on mismatch

**Tests:**
- [ ] Exact value match passes
- [ ] Wrong value produces clear mismatch message: `"expected response.body.email to be 'a@b.com', got 'x@y.com'"`
- [ ] Missing path in actual output produces message
- [ ] Nested path extraction works
- [ ] Partial matching — extra fields don't cause failure
- [ ] Array comparison works
- [ ] Status mismatch detected

**Acceptance criteria:** Expectations are matched with clear mismatch explanations.

---

## Task 7.5: Test Result Formatting

**Description:** Format test results for CLI output.

**Subtasks:**

- [ ] Create `internal/testing/format.go`
- [ ] Implement `FormatResults(suiteResults []SuiteResult) string`:
  - For each suite: show workflow name
  - For each case: `✓ case name` (green) or `✗ case name` (red) with failure reason indented
  - Summary: `"X passed, Y failed, Z total"`
- [ ] Verbose mode (`--verbose`): also show execution trace for each test case — node execution order, timing, output data at each step
- [ ] Color support: detect terminal, use green/red if supported

**Tests:**
- [ ] Passing suite formats correctly
- [ ] Failed case shows failure reason
- [ ] Summary counts are accurate
- [ ] Verbose mode shows trace
- [ ] Non-color output is readable

**Acceptance criteria:** Test output is clear and actionable.

---

## Task 7.6: Wire `noda test` Command

**Description:** Connect the test runner to the CLI.

**Subtasks:**

- [ ] Update `cmd/noda/` to replace placeholder `test` command with real implementation
- [ ] `noda test`:
  - Load config via `ValidateAll()`
  - Load all test suites
  - Run all test suites
  - Print results
  - Exit 0 if all pass, exit 1 if any fail
- [ ] `noda test --workflow <id>`:
  - Run only test suites for the specified workflow
- [ ] `noda test --verbose`:
  - Show execution traces for all test cases (not just failures)
- [ ] Handle: no test files found → print "No test files found in tests/", exit 0

**Tests:**
- [ ] `noda test` on project with passing tests → exit 0
- [ ] `noda test` on project with failing tests → exit 1 with failure details
- [ ] `noda test --workflow create-user` runs only that suite
- [ ] `noda test --verbose` shows traces
- [ ] No test files → exit 0 with message

**Acceptance criteria:** `noda test` is fully functional from the CLI.

---

## Task 7.7: Sample Test Suites

**Description:** Create comprehensive test fixtures that exercise the testing framework.

**Subtasks:**

- [ ] Create `testdata/valid-project/tests/test-create-task.json`:
  - Test case: successful creation — mock db.create returns task, expect 201 response
  - Test case: validation failure — input fails transform.validate, expect error path
  - Test case: database failure — mock db.create errors, expect error handling
- [ ] Create `testdata/valid-project/tests/test-list-tasks.json`:
  - Test case: list with results — mock parallel db.query nodes, expect paginated response
  - Test case: empty list — mock returns empty array
- [ ] Create `testdata/valid-project/tests/test-get-task.json`:
  - Test case: task found — mock db.query returns task, expect 200
  - Test case: task not found — mock db.query returns empty, expect 404 via control.if
- [ ] All test files use the valid-project workflows from M1's test fixtures

**Tests:**
- [ ] All sample tests pass when run through the framework
- [ ] Sample tests demonstrate all mock patterns (success, error, workflow.run output name)

**Acceptance criteria:** Realistic test suites demonstrate and validate the testing framework.
