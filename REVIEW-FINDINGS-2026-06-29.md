# Noda Backend Security & Cleanliness Review — Findings

Date: 2026-06-29  
Branch: main (commit 4e2213b)  
Scope: Go backend (`internal/`, `plugins/`, `pkg/`, `cmd/`) + repo hygiene. Editor frontend, example projects, and performance benchmarking out of scope.  
Method: Tooling-grounded multi-agent review — automated baseline (golangci-lint+gosec, staticcheck, go vet, govulncheck, secret/history & hygiene scans) → 12 parallel domain-review agents → adversarial refute-pass verifying each finding against the actual code and vendored library source.  
Status: **Verified.** 27 of 30 candidate findings survived adversarial verification (3 refuted — see appendix). No Critical or High findings survived. Closed 2026-04 items (C1–C3, H5/H6/H9–H11/H13/H15/H16) were excluded by construction.  
Spec/plan: `docs/superpowers/specs|plans/2026-06-29-backend-security-review*`

**Summary:** 12 Medium, 15 Low — 12 security, 14 correctness, 1 hygiene. All findings are **Open**.

---

## Medium

### edge-io-1. IPv6 cloud-metadata endpoints reachable when allow_private_networks=true, breaking the "metadata is uncircumventable" guarantee
- **Files:** `internal/netguard/netguard.go:35-38`
- **Unit / dimension:** edge-io / security  (reviewer confidence 0.5)
- **Claim:** netguard's metadataIPs list contains only the two IPv4 metadata addresses; IPv6 cloud-metadata endpoints (AWS fd00:ec2::254, GCP link-local fe80::a9fe:a9fe) are blocked only by privateBlocks (fc00::/7, fe80::/10), which allow_private_networks lifts, so SSRF can reach IPv6 IMDS despite the code's explicit promise that metadata stays blocked.
- **Evidence:** var metadataIPs = []net.IP{
	net.ParseIP("169.254.169.254"), // AWS, GCP, Azure, DO, Oracle, IBM, OpenStack
	net.ParseIP("100.100.100.200"), // Alibaba Cloud
}  // comment at line 33: "metadataIPs are uncircumventable: they remain blocked even when AllowPrivateNetworks is true". checkHostWithLookup line 128: `if !p.AllowPrivateNetworks && !hostAllowed && ipInBlocks(ip, privateBlocks)` — fc00::/7 and fe80::/10 are in privateBlocks, so allow_private_networks=true admits fd00:ec2::254 and fe80::a9fe:a9fe; ipIsMetadata never matches them.
- **Suggested fix:** Add the IPv6 metadata addresses to metadataIPs: net.ParseIP("fd00:ec2::254") (AWS IMDS IPv6) and net.ParseIP("fe80::a9fe:a9fe") (GCP/Azure IPv6 link-local metadata). Since metadataIPs is checked before the AllowPrivateNetworks bypass in both checkHostWithLookup and ipDenied, this restores the uncircumventable guarantee for IPv6.
- **Verification:** Confirmed in internal/netguard/netguard.go. metadataIPs (lines 35-38) lists only IPv4 addresses, and ipIsMetadata (lines 81-88) matches only those literals. The IPv6 cloud-metadata endpoints fd00:ec2::254 (within fc00::/7, line 49) and fe80::a9fe:a9fe (within fe80::/10, line 45) are covered exclusively by privateBlocks. Both checkHostWithLookup (line 128) and ipDenied (line 155) gate the privateBlocks check behind !p.AllowPrivateNetworks, so allow_private_networks=true admits the IPv6 metadata addresses, directly contradicting the code's documented promise (lines 33-34) that metadata IPs stay blocked even when AllowPrivateNetworks is true. The gap is real and reachable on dual-stack hosts with IPv6 IMDS enabled. Severity lowered to M (not H/C) because exploitation requires the operator to have explicitly enabled allow_private_networks (itself an opt-out of private-network protection) plus an IPv6-reachable metadata service; it is a defense-in-depth invariant violation rather than a default-config SSRF.
- **Status:** Open

### execution-1. TimeoutMiddleware runs handler in a child goroutine, so panics escape RecoverMiddleware and crash the process
- **Files:** `internal/worker/middleware.go:84-100`
- **Unit / dimension:** execution / correctness  (reviewer confidence 0.7)
- **Claim:** Because TimeoutMiddleware executes the downstream handler chain in a separate goroutine without its own recover, a panic raised inside that goroutine is not caught by an outer RecoverMiddleware (recover only works on the same goroutine), so it propagates and aborts the whole worker process.
- **Evidence:** done := make(chan error, 1)
		go func() {
			done <- next(ctx)
		}()
		select {
		case err := <-done:
			return err
		case <-ctx.Done(): ...  // DefaultMiddleware order is {Recover, Log, Timeout}, so Recover wraps Timeout — its deferred recover() sits on the PARENT goroutine and cannot catch a panic from next(ctx) running in the spawned goroutine
- **Suggested fix:** Add a deferred recover() inside the spawned goroutine that converts the panic into an error sent on the done channel (e.g. defer func(){ if r:=recover(); r!=nil { done <- fmt.Errorf("panic: %v", r) } }()), so panics are surfaced as errors rather than crashing the process.
- **Verification:** Confirmed against the code. middleware.go:84-87 spawns `next(ctx)` in a child goroutine; RecoverMiddleware (middleware.go:109-124) installs its `recover()` on the parent goroutine. runtime.go:304-314 applies middleware in reverse, so DefaultMiddleware {Recover, Log, Timeout} composes as Recover(Log(Timeout(handler))) — exactly as the finding claims. Go's recover() only catches panics on the same goroutine, so a panic inside the Timeout-spawned goroutine cannot be caught by RecoverMiddleware and would crash the worker process. Partial mitigation exists: internal/engine/dispatch.go:21-26 has its own deferred recover() inside dispatchNode, so panics raised inside a node executor's Execute() are converted to errors and do NOT escape. However, engine orchestration code in ExecuteGraph (executor.go, e.g. edge/adjacency map handling around lines 147-189, metrics, retry plumbing) runs outside dispatchNode's recover, so a panic there in the Timeout-spawned goroutine is unrecovered and aborts the process. The RecoverMiddleware safety net is genuinely defeated for everything except node-executor bodies. Reachable but requires a panic outside dispatchNode, hence Medium rather than High. The suggested fix (deferred recover inside the spawned goroutine) is appropriate.
- **Status:** Open

### execution-3. Scheduler lock key is hard-coded to 1-minute granularity, silently dropping sub-minute scheduled fires
- **Files:** `internal/scheduler/runtime.go:229`
- **Unit / dimension:** execution / correctness  (reviewer confidence 0.6)
- **Claim:** The distributed lock key buckets the fire time with now.Truncate(time.Minute), but WithSeconds() allows schedules that fire more than once per minute. With locking enabled, the 2nd+ fire within the same minute computes the same lock key, the lock is still held (TTL >= timeout+30s and never released), so those fires are skipped — missed executions for any sub-minute schedule.
- **Evidence:** lockKey := fmt.Sprintf("noda:schedule:%s:%d", sc.ID, now.Truncate(time.Minute).Unix())  // ... Do NOT release the lock after execution ... must be held until the TTL expires
- **Suggested fix:** Derive the lock-window granularity from the schedule's actual interval (e.g. truncate to the cron period or use the cron entry's scheduled Next time) instead of a fixed time.Minute, so each distinct fire gets a distinct lock key.
- **Verification:** Confirmed in code. runtime.go:108 enables cron.WithSeconds(), so sub-minute schedules are valid. The lock key at line 229 truncates fire time to time.Minute. lockTTL is forced to >= timeout+30s (default >=5m30s) at lines 224-227, and lock.go:29-31 defaults a zero TTL to 5min, so the lock always outlives the 1-minute bucket. Lines 277-280 deliberately never release the lock. Thus a sub-minute schedule (e.g. '*/30 * * * * *') with LockEnabled has its 2nd+ fire within the same minute hit the identical lock key while the lock is still held; tryAcquireLock returns "" and the run is recorded as Skipped (lines 263-275). Genuine missed-execution defect. Downgraded to Medium because it only manifests when both a sub-minute cron and distributed locking are configured.
- **Status:** Open

### execution-4. Worker consume loop has no panic recovery outside the middleware/engine, so a pre-handler panic permanently kills a consumer
- **Files:** `internal/worker/runtime.go:223-261`
- **Unit / dimension:** execution / correctness  (reviewer confidence 0.55)
- **Claim:** processMessage performs payload deserialization, input-mapping resolution, and middleware-chain construction before the RecoverMiddleware-wrapped handler runs, and the consume goroutine itself has no recover. A panic in any of that pre-handler code (or when no recover middleware is configured) propagates out of consume, runs defer r.wg.Done(), and terminates that consumer goroutine for good — silently reducing the worker's concurrency with no restart. The scheduler's runJob has a top-level recover but the worker does not.
- **Evidence:** func (r *Runtime) consume(ctx context.Context, ...) {
	defer r.wg.Done()
	for { ... r.processMessage(ctx, w, client, consumerID, msg) ... } }  // no recover; input, err := engine.ResolveInput(...) and deserializePayload run before the middleware chain
- **Suggested fix:** Add a defer/recover at the top of processMessage (mirroring scheduler.runJob) that logs the panic and nacks/leaves the message pending, so a single bad message cannot permanently remove a consumer.
- **Verification:** Verified against the code. internal/worker/runtime.go consume() (177-214) has only `defer r.wg.Done()` and no recover; it calls processMessage directly (210). In processMessage (223-358), deserializePayload, engine.ResolveInput, NewExecutionContext, and the middleware Wrap construction loop (lines 250-314) all run BEFORE handler(procCtx) at line 317. RecoverMiddleware (middleware.go:109-124) only wraps the handler invocation, so a panic in that pre-handler setup propagates out of consume, runs wg.Done(), and permanently kills that consumer goroutine with no restart, silently reducing concurrency. Additionally, ResolveMiddleware (middleware.go:136-151) lets a worker config supply a custom chain with no worker.recover at all, leaving the handler unprotected too. The asymmetry with scheduler.runJob (runtime.go:188-203), which has a top-level recover, is confirmed. The suggested fix (top-level recover in processMessage mirroring runJob) is valid. Corrected to Medium rather than High because the dominant panic surface — node/plugin execution — is already recovered upstream in engine/dispatch.go dispatchNode (21-26), so the realistically reachable escape path (pre-handler setup on JSON-unmarshalled payload, ExecuteGraph orchestration, or a misconfigured middleware chain) is narrow; this is an availability/defense-in-depth gap, not a direct security vuln.
- **Status:** Open

### nodes-api-1. util.jwt_sign accepts an empty signing secret, producing trivially forgeable tokens
- **Files:** `plugins/core/util/jwt.go:60-68,122-123`
- **Unit / dimension:** nodes-api / security  (reviewer confidence 0.5)
- **Claim:** If the resolved secret is an empty string, jwt_sign signs the token with an empty HMAC key instead of rejecting it, yielding a forgeable token.
- **Evidence:** secret, ok := secretVal.(string)
if !ok {
    return "", nil, fmt.Errorf("util.jwt_sign: secret must resolve to a string, got %T", secretVal)
}
...
signed, err := token.SignedString([]byte(secret))  // empty []byte is accepted by golang-jwt HMAC and produces a valid signature
- **Suggested fix:** After resolving, reject empty secrets: `if secret == "" { return "", nil, fmt.Errorf("util.jwt_sign: secret resolved to empty string") }`. Optionally enforce a minimum length for HS256/384/512.
- **Verification:** Confirmed against code and library. plugins/core/util/jwt.go:65-68 only verifies the resolved secret is a string and has no empty-string check; line 123 calls token.SignedString([]byte(secret)). The vendored golang-jwt/jwt/v5@v5.3.1 hmac.go Sign method accepts an empty []byte key with no length validation, computing a valid HMAC and returning a valid signature. Thus a secret expression resolving to "" (e.g. an unset env var) silently yields a forgeable token. Reachable via misconfiguration; no upstream guard exists. Severity reduced to Medium since it requires an empty-secret misconfiguration rather than being exploitable on a properly configured deployment.
- **Status:** Open

### platform-1. Shutdown signal during StartAll is silently ignored (no graceful stop of components started after the signal)
- **Files:** `internal/lifecycle/lifecycle.go:51-76, 83-89`
- **Unit / dimension:** platform / correctness  (reviewer confidence 0.55)
- **Claim:** StartAll only sets l.started=n after the whole start loop completes, and holds no lock across it, so a concurrent StopAll (signal handler) observes started=0 and stops nothing.
- **Evidence:** StartAll: `for i, c := range components { ... if err := c.Start(ctx); ... } l.mu.Lock(); l.started = n; l.mu.Unlock()` — started is 0 for the entire duration of the start loop. StopAll: `l.mu.Lock(); started := l.started; components := make([]Component, started); copy(...); l.started = 0; l.mu.Unlock(); if started == 0 { return }`. The signal handler in cmd/noda/runtime.go runs `lc.StopAll(shutdownCtx); close(doneCh)` in a goroutine installed before StartAll is called, so a SIGINT/SIGTERM arriving while StartAll is still connecting services (common during k8s rolling deploys) makes StopAll a no-op, closes doneCh, and the process tears down without gracefully stopping the HTTP server / workers / services that StartAll continues to bring up.
- **Suggested fix:** Track started components incrementally under the mutex (increment l.started after each successful Start inside the loop, not only at the end), or guard StartAll/StopAll with a shared state machine so a stop requested mid-start is observed and drains the components started so far.
- **Verification:** Confirmed against the actual code. In internal/lifecycle/lifecycle.go, StartAll (lines 51-76) copies components under the mutex, releases the lock, then runs the start loop holding no lock; l.started remains 0 for the entire loop and is only set to n at line 73 after completion (or to i at line 63 on error). StopAll (lines 83-93) reads started=l.started and returns immediately when started==0 (lines 91-93). In cmd/noda/runtime.go the signal handler goroutine (lines 295-307) calling lc.StopAll(shutdownCtx) then close(doneCh) is installed at lines 294-307 BEFORE lc.StartAll at line 335, and StartAll receives context.Background() (uncancellable). Therefore a SIGINT/SIGTERM arriving mid-StartAll makes StopAll observe started=0 and no-op, closes doneCh, and StartAll continues bringing components up that are then never gracefully Stop()'d. No upstream guard exists (no shutdown-requested flag, no incremental started tracking). The race and leak are exactly as described. Severity lowered to M: the window is limited to the StartAll loop duration and the process is exiting regardless, but the graceful-shutdown contract (drain/clean close of services and connections) is genuinely violated.
- **Status:** Open

### realtime-1. WebSocket broadcast head-of-line blocking: SendFn writes with no deadline/buffer, one stuck client stalls the whole channel
- **Files:** `internal/connmgr/websocket.go:128-132 (SendFn); manager.go:157-163 (sequential Send loop)`
- **Unit / dimension:** realtime / security  (reviewer confidence 0.78)
- **Claim:** A single slow or non-reading WebSocket client blocks delivery to every other client on the channel (and stalls pings) for up to 2x PingInterval, because SendFn does a synchronous ws.WriteMessage with no write deadline and no per-conn outbound queue, and Manager.Send delivers sequentially while one call holds wsMu.
- **Evidence:** SendFn: `SendFn: func(data []byte) error { wsMu.Lock(); defer wsMu.Unlock(); return ws.WriteMessage(websocket.TextMessage, data) }` — no SetWriteDeadline. Manager.Send: `for _, conn := range conns { if conn.SendFn != nil { if err := conn.SendFn(payload); err != nil {...} } }` iterates serially. If a client stops reading, its TCP send buffer fills and ws.WriteMessage blocks indefinitely, holding wsMu (so the ping goroutine at websocket.go:170 also blocks) and halting the broadcast loop. SSE deliberately avoids this with a non-blocking bounded.Queue (sse.go:96-135), but the WebSocket path was not given the same treatment.
- **Suggested fix:** Mirror the SSE design: give each WS connection a bounded outbound queue drained by a single writer goroutine, OR at minimum set a short write deadline before each write (e.g. `ws.SetWriteDeadline(time.Now().Add(5*time.Second))` immediately before WriteMessage in SendFn) so a stuck client is dropped rather than blocking the whole channel.
- **Verification:** Confirmed against code and vendored library. websocket.go:128-132 SendFn does ws.WriteMessage under wsMu with no SetWriteDeadline. The underlying fasthttp/websocket Conn (conn.go:251,417) defaults writeDeadline to the zero time.Time, i.e. NO deadline, so WriteMessage blocks until the TCP send buffer drains — indefinitely for a non-reading client. Manager.Send (manager.go:157-163) delivers serially, calling each conn.SendFn inline, so one stuck client halts delivery to all other clients on the channel. The ping goroutine (websocket.go:170-172) grabs the same wsMu before WriteControl, so it is also blocked. The SSE path deliberately avoids this with a non-blocking bounded.Queue (sse.go:95-135); the WS path lacks any equivalent or write timeout (websocket.Config sets only buffer sizes). Real head-of-line-blocking / availability defect. Note the claim's '2x PingInterval' bound is imprecise — a deadline-less write isn't released by read-deadline expiry, so blocking can persist far longer (until TCP RST/keepalive), making the impact if anything worse than stated. Severity Medium: reliability/DoS-style degradation, not a direct security exploit.
- **Status:** Open

### server-1. Route-group middleware selection is non-deterministic and never composes overlapping groups
- **Files:** `internal/server/presets.go:106-127`
- **Unit / dimension:** server / security  (reviewer confidence 0.7)
- **Claim:** getGroupMiddleware ranges over a Go map (random order) and returns the FIRST prefix match, so when several route_groups prefixes match a path, which group's auth/authz middleware applies is non-deterministic, and middleware from the more-specific group is silently dropped.
- **Evidence:** getRouteGroups() returns map[string]map[string]any; getGroupMiddleware then does: `for prefix, group := range groups { if strings.HasPrefix(routePath, prefix) { ... return s.expandPreset(preset) ... return result, nil } }`. Go randomizes map iteration order and the function returns on the first matching prefix. For path "/api/admin/users" with groups "/api" (e.g. [auth.jwt]) and "/api/admin" (e.g. [auth.jwt, casbin.enforce]), both prefixes match; the chosen group varies per process start, and only one group's chain is ever applied so casbin enforcement can be dropped.
- **Suggested fix:** Collect all matching prefixes, sort by descending length (most-specific first) for deterministic selection, and either pick the longest match or merge all matching groups' middleware in a defined order before dedupe. Do not rely on map iteration order.
- **Verification:** Confirmed against internal/server/presets.go:106-139. getRouteGroups() returns map[string]map[string]any (line 129), and getGroupMiddleware ranges over that map (line 108) and returns on the FIRST strings.HasPrefix match. Go randomizes map iteration order, so when several route_groups prefixes match a path (e.g. "/api" and "/api/admin" both matching "/api/admin/users"), which group's middleware chain applies is non-deterministic, and the non-selected (often more-specific) group's chain — including casbin.enforce — is silently dropped. No upstream guard prevents overlapping prefixes: ValidatePresets only checks that preset names exist, not prefix overlap. The behavior is in fact explicitly acknowledged as a known limitation in docs/02-config/middleware.md:46 ("the winner is non-deterministic (Go map iteration)... Define disjoint prefixes rather than nested ones"). The defect and its security consequence (potential authz bypass) genuinely exist and are reachable via the intuitive nested-prefix configuration; the suggested fix (collect all matches, sort by descending prefix length, pick longest or merge deterministically) is valid. Severity corrected to Medium rather than High because exploitation requires operator misconfiguration that the docs warn against, though nested prefixes with most-specific-wins is an intuitive expectation that makes accidental misconfiguration plausible.
- **Status:** Open

### server-2. JWT middleware does not validate audience/issuer and accepts tokens with no expiry
- **Files:** `internal/server/middleware.go:391-405`
- **Unit / dimension:** server / security  (reviewer confidence 0.7)
- **Claim:** newJWTMiddleware verifies signature and (if present) exp, but never checks the aud audience or iss issuer claims, and does not require an exp claim, so a validly-signed token minted for a different audience, or a token with no expiry, is accepted indefinitely.
- **Evidence:** `token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) { if t.Method.Alg() != signingMethod.Alg() {...}; return verifyKey, nil })`. No jwt.WithAudience(...), jwt.WithIssuer(...), or jwt.WithExpirationRequired() options are passed (grep confirms none exist in the package), and there are no expected_audience/issuer config fields. golang-jwt v5 only validates exp when present, so a token omitting exp passes. With a shared HMAC secret this allows cross-service token reuse and non-expiring tokens.
- **Suggested fix:** Add optional audience and issuer config fields and pass jwt.WithAudience(aud), jwt.WithIssuer(iss), and jwt.WithExpirationRequired() to jwt.Parse; reject tokens whose aud/iss mismatch or that lack exp.
- **Verification:** Confirmed. internal/server/middleware.go:391 (newJWTMiddleware) calls jwt.Parse with only a keyfunc that checks the alg; no parser options are passed and grep shows no WithAudience/WithIssuer/WithExpirationRequired in the package's auth.jwt path, nor any audience/issuer config fields. I read the vendored golang-jwt v5.3.1 validator.go: requireExp defaults to false (verifyExpiresAt with require=false passes when exp is absent), audience is only validated when len(expectedAud)>0, and issuer only when expectedIss!='' — none of which are set here. Therefore a validly-signed token with no exp is accepted indefinitely and aud/iss are never validated. The OIDC middleware (oidc.go) does validate aud/iss, but the HMAC/RSA/ECDSA auth.jwt middleware does not, and there is no upstream guard. Defect is real and reachable. Severity Medium: the cross-service-reuse and non-expiring-token impact depends on shared secrets / token leakage, making it a defense-in-depth hardening gap rather than a direct unauthenticated bypass.
- **Status:** Open

### server-3. Route-group prefix matching is not path-segment aware
- **Files:** `internal/server/presets.go:109`
- **Unit / dimension:** server / security  (reviewer confidence 0.55)
- **Claim:** Group matching uses strings.HasPrefix on the raw path, so a group prefix like "/admin" also matches unrelated paths such as "/administration-public" and "/api" matches "/api-docs", causing group middleware to be applied to (or omitted from) routes the operator did not intend.
- **Evidence:** `for prefix, group := range groups { if strings.HasPrefix(routePath, prefix) {`. There is no segment boundary check: prefix "/api" matches "/apifoo" and "/api-docs". Combined with first-match-wins (server-1), this can attach the wrong group's auth chain to a route or skip the intended one.
- **Suggested fix:** Match on path segment boundaries: require routePath == prefix or strings.HasPrefix(routePath, strings.TrimSuffix(prefix, "/")+"/"), so "/api" matches "/api" and "/api/..." but not "/api-docs".
- **Verification:** Confirmed against the code: presets.go:109 reads `if strings.HasPrefix(routePath, prefix)` inside `getGroupMiddleware`, iterating route-group prefixes pulled verbatim from config (getRouteGroups, lines 129-139). There is no path-segment boundary check anywhere, so prefix `/api` matches `/api-docs`/`/apifoo` and `/admin` matches `/administration-public`. Group middleware can include auth/casbin chains via presets (lines 110-123), so the wrong group's middleware can be applied to unintended routes. The defect genuinely exists and is reachable. Rated M (not H) because it requires an operator to configure prefixes that are string-prefixes of unrelated paths, and the over-match direction typically adds middleware rather than removing auth; the wrong-chain risk arises mainly with nondeterministic map iteration (first-match-wins).
- **Status:** Open

### wasm-1. Wasm execution is not interruptible: runaway/malicious module pegs a CPU core and leaks a goroutine despite callWithTimeout
- **Files:** `internal/wasm/runtime.go:66-92 (manifest) + internal/wasm/module.go:417-442 (callWithTimeout)`
- **Unit / dimension:** wasm / security  (reviewer confidence 0.88)
- **Claim:** No fuel/instruction limit and no real execution timeout: the Extism manifest never sets Timeout, so wazero is not configured to interrupt on context-done, and Plugin.Call runs on context.Background(); callWithTimeout only abandons the wait while the wasm keeps running forever.
- **Evidence:** runtime.go builds 'manifest := extism.Manifest{Wasm:..., AllowedHosts: cfg.AllowHTTP}' (+ optional Memory) with no Timeout field. module.go callWithTimeout: 'go func(){ ...; exitCode,output,err := m.Plugin.Call(name,data); ch <- ... }()' and on '<-timer.C' returns 'call timed out' while that goroutine keeps executing. Extism go-sdk v1.7.1 plugin.go: 'if manifest.Timeout > 0 { runtimeConfig = runtimeConfig.WithCloseOnContextDone(true) }', and Plugin.Call -> CallWithContext(context.Background(),...). With Timeout unset, an infinite-loop guest is never interrupted: the tracked goroutine never returns, outstandingCalls.Done() never fires (Stop's 5s wait times out and leaks), and a CPU core is pinned permanently.
- **Suggested fix:** Set manifest.Timeout (timeout_ms, derived from cfg.TickTimeout/wasmCallTimeout) so wazero enables WithCloseOnContextDone, and call the plugin via a context-carrying CallWithContext bound to a per-call timeout / m.lifecycleCtx so a runaway guest is actually halted. Consider wazero fuel/epoch interruption as defense in depth.
- **Verification:** Verified against code and vendored extism go-sdk v1.7.1. runtime.go:66-77 builds extism.Manifest with no Timeout field. plugin.go:129-131 only enables wazero WithCloseOnContextDone when manifest.Timeout>0; extism.go:448-449 Plugin.Call uses context.Background(); extism.go:459-462 CallWithContext only adds a deadline when p.Timeout(=manifest.Timeout)>0. Therefore the guest f.Call runs on a non-cancelable Background context and wazero never interrupts it. module.go:417-442 callWithTimeout spawns a goroutine and only abandons the wait on timer/lifecycleCtx; cancelling lifecycleCtx has no effect on the running guest. An infinite-loop or CPU-heavy guest export pins a core permanently and outstandingCalls.Done() never fires, so Stop's 5s outstandingCalls.Wait (module.go:216-223) times out and leaks the goroutine. Wasm modules are the documented custom-logic extension, so this is reachable. The 'callWithTimeout' name gives a false guarantee of bounded execution.
- **Status:** Open

### wasm-3. Gateway reconnectLoop ignores stop/close, resurrecting and leaking an outbound WebSocket after module Stop or ws_close
- **Files:** `internal/wasm/gateway.go:326-375`
- **Unit / dimension:** wasm / correctness  (reviewer confidence 0.78)
- **Claim:** reconnectLoop never checks gc.stopCh / gc.closed during its retry loop, so a reconnect already in flight when Stop()/CloseAll() or CloseConn() runs will re-dial, set gc.closed=false, replace gc.ws, and spawn a new readLoop on a module that has been torn down — leaking an untracked live connection.
- **Evidence:** reconnectLoop: 'for attempt := 1; attempt <= rcfg.MaxAttempts; attempt++ { time.Sleep(delay); conn,_,err := websocket.DefaultDialer.Dial(gc.url, gc.headers); ... gc.ws = conn; gc.closed = false; gc.stopCh = make(chan struct{}); ... go g.readLoop(gc) }'. CloseAll sets 'gc.closed = true; close(gc.stopCh)' and CloseConn likewise, but neither value is consulted inside the attempt loop, and the loop is spawned from readLoop's defer ('go g.reconnectLoop(gc)') on any drop, including drops racing with shutdown. The resurrected gc is not re-added to g.conns, so it is never closed again and its readLoop keeps buffering into m.AddIncomingWS.
- **Suggested fix:** At the top of each attempt (and before/after the sleep and after a successful dial) check 'select { case <-gc.stopCh: return; default: }' / gc.closed under lock; abort reconnection if the connection or module is closing, and close any conn dialed after a stop was observed.
- **Verification:** Verified in internal/wasm/gateway.go. reconnectLoop (lines 326-378) never consults gc.stopCh or gc.closed inside its attempt loop; it sleeps (>=1s window) then re-dials and on success unconditionally sets gc.ws=conn, gc.closed=false, gc.stopCh=make(chan struct{}), resets closeOnce, and spawns a new readLoop. reconnectLoop is launched from readLoop's defer on any natural drop (when wasClosed==false). CloseConn (line 144) deletes gc from g.conns and sets closed=true/close(stopCh); CloseAll (line 218, invoked from module.Stop at module.go:236) does likewise on the whole map. If either runs while a reconnectLoop spawned by an earlier drop is sleeping, the loop wakes, re-dials, resurrects the connection, and spawns a new readLoop on a gc that is no longer in g.conns — so it can never be closed again and keeps feeding m.AddIncomingWS on a torn-down module. Reachable whenever reconnect.enabled is configured (via Configure). Genuine concurrency leak.
- **Status:** Open

---

## Low

### db-1. Raw database error message leaked to HTTP clients via ConflictError.Reason
- **Files:** `plugins/db/create.go:77-83`
- **Unit / dimension:** db / security  (reviewer confidence 0.75)
- **Claim:** On unique/duplicate-key violations, db.create and db.upsert put the raw driver error string into ConflictError.Reason, which the server returns verbatim to clients (HTTP 409) even in production.
- **Evidence:** create.go: `errMsg := tx.Error.Error()` ... `return "", nil, &api.ConflictError{Resource: table, Reason: errMsg}` (identical in upsert.go:92-98). pkg/api/errors.go: `func (e *ConflictError) Error() string { return fmt.Sprintf("conflict on %s: %s", e.Resource, e.Reason) }`. internal/server/errors.go:57-65 maps ConflictError unconditionally: `Message: cfErr.Error()` at status 409 — unlike the default 500 path which hides the message unless devMode. The Postgres/SQLite message discloses internal constraint/index/column names (e.g. `duplicate key value violates unique constraint "users_email_key"`) and confirms existence of a conflicting record, enabling schema disclosure and account/value enumeration.
- **Suggested fix:** Do not embed the raw driver error in a client-facing field. Set Reason to a generic message (e.g. "resource already exists") or omit it, and log the full tx.Error server-side only. Alternatively gate Reason behind devMode like the default 500 branch.
- **Verification:** Confirmed in code. plugins/db/create.go:77-83 and upsert.go:91-98 set ConflictError.Reason = tx.Error.Error() (raw driver message) on unique/duplicate-key violations. pkg/api/errors.go:62-64 interpolates Reason into ConflictError.Error(). internal/server/errors.go:57-65 maps ConflictError to HTTP 409 with Message: cfErr.Error() with NO devMode guard, unlike the default 500 branch (lines 84-96) which hides err.Error() unless devMode. Therefore raw Postgres/SQLite messages (e.g. constraint/index names like users_email_key) are returned verbatim to clients in production, leaking schema details and confirming conflicting-record existence. Reachable on any db.create/db.upsert against a uniquely-constrained table; no upstream mitigation. Severity downgraded to Low: it is genuine information disclosure but limited to constraint-name leakage and existence confirmation, not direct data/credential exposure.
- **Status:** Open

### db-2. ValidateSQLFragment is a keyword blocklist that permits boolean-logic injection in interpolated raw fragments
- **Files:** `plugins/db/validate.go:79-97`
- **Unit / dimension:** db / security  (reviewer confidence 0.5)
- **Claim:** where_clause/having/join-`on` query strings are expression-interpolated via nCtx.Resolve before validation, and ValidateSQLFragment only blocks `;`, comments, and a fixed keyword list — so request data interpolated into a fragment string can still inject boolean/predicate logic (e.g. `OR 1=1`) without using any blocked token.
- **Evidence:** where.go:43-53 resolves then validates: `resolved, err := nCtx.Resolve(queryStr)` ... `ValidateSQLFragment(query)`. validate.go blocklist = {DROP,DELETE,INSERT,UPDATE,ALTER,CREATE,EXEC,UNION,SELECT,GRANT,REVOKE,TRUNCATE}; a fragment like `name = '${request.query.name}'` with name=`x' OR '1'='1` passes all checks (no `;`, no `--`, no blocked keyword). The intended-safe `params` mechanism exists, so this is a defense-in-depth gap rather than a guaranteed exploit, but the Resolve-then-blocklist design actively enables developer interpolation of untrusted input into raw SQL.
- **Suggested fix:** Document and lint against interpolating expressions into fragment query strings (params only); consider rejecting fragments whose resolved value differs from the static template, or move to an allowlist/AST-based validation for raw fragments.
- **Verification:** Confirmed against the code. where.go:43-53 calls nCtx.Resolve(queryStr) on where_clause.query BEFORE ValidateSQLFragment, and Resolve performs string interpolation: internal/expr/evaluator.go:30-46 concatenates expression results into surrounding literal text via fmt.Fprintf("%v"). So a developer-authored fragment like name = '{{ request.query.name }}' resolves untrusted request data into the SQL string before validation. ValidateSQLFragment (validate.go:79-97) is a pure blocklist that only rejects ;, --, /*, and the whole-word keywords {DROP,DELETE,INSERT,UPDATE,ALTER,CREATE,EXEC,UNION,SELECT,GRANT,REVOKE,TRUNCATE}. A value such as x' OR '1'='1 contains none of these tokens, so the resolved string name = 'x' OR '1'='1' passes and is handed to tx.Where (where.go:174); the same resolve-then-blocklist pattern applies to joins[].on (line 110) and having (lines 235/253). The defect is genuine and the blocklist demonstrably fails to stop boolean-logic injection. Severity corrected to Low: it is a defense-in-depth weakness, not a default exploit — it requires the config author to interpolate untrusted input into a raw fragment instead of using the provided, intended-safe params binding (the params path uses parameterized GORM placeholders). The finding's only inaccuracy is cosmetic (it cites ${...} syntax; noda uses {{ }}), which does not affect the substance.
- **Status:** Open

### db-3. db.upsert returns the input data, dropping server-generated columns
- **Files:** `plugins/db/upsert.go:90-102`
- **Unit / dimension:** db / correctness  (reviewer confidence 0.6)
- **Claim:** Unlike db.create (which uses clause.Returning{} and repopulates id/created_at), db.upsert returns the original input `data` map, so generated columns (id, created_at, updated_at) are absent from the node output despite the descriptor advertising "The upserted row object".
- **Evidence:** upsert.go: `tx := db.WithContext(ctx).Table(table).Clauses(onConflict).Create(row)` ... `return api.OutputSuccess, data, nil` — `data` is the pre-insert input; no Returning clause is added, so downstream nodes referencing the generated id get nothing.
- **Suggested fix:** Add clause.Returning{} to the upsert and return the repopulated row (restoring JSON composites as create.go does), or update the OutputDescriptions to state generated fields are not returned.
- **Verification:** Confirmed against the code. plugins/db/upsert.go line 90 issues `db.WithContext(ctx).Table(table).Clauses(onConflict).Create(row)` with no clause.Returning{}, and line 102 returns `data` (the pre-insert ResolveMap input from line 65), which is never repopulated from the DB. So server-generated columns (id, created_at, updated_at) are absent from the success output. This directly contrasts with plugins/db/create.go line 75 which adds clause.Returning{} and returns the repopulated `row` (lines 91-97). The upsert OutputDescriptions (line 37) advertises "The upserted row object", while create explicitly documents generated fields are included (line 35) — confirming the intended contract that upsert breaks. Defect is real and reachable on every upsert. It is a data-completeness/behavioral inconsistency with no security impact, so severity Low rather than higher.
- **Status:** Open

### edge-io-2. storage validatePath does not reject absolute paths (incomplete defense-in-depth)
- **Files:** `plugins/storage/service.go:17-23`
- **Unit / dimension:** edge-io / security  (reviewer confidence 0.4)
- **Claim:** validatePath, documented as rejecting path-traversal as defense-in-depth, only rejects ".."-style escapes and lets absolute paths (e.g. "/etc/passwd") through; exploitation is currently prevented only by afero BasePathFs re-rooting, so the in-package guard is weaker than its docstring claims.
- **Evidence:** func validatePath(path string) error {
	cleaned := filepath.ToSlash(filepath.Clean(path))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("storage: path traversal not allowed: %q", path)
	}
	return nil
}  // "/etc/passwd" -> Clean -> "/etc/passwd", not ".." and no "../" prefix, so it passes. Only afero.NewBasePathFs (plugin.go:45) joins it back under the base and confines it.
- **Suggested fix:** Reject absolute paths in validatePath as well, e.g. add `if filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, "/") { return fmt.Errorf(...) }`, mirroring pathutil.ValidateRelative which already rejects absolute paths and NUL bytes. Reusing pathutil.ValidateRelative here would unify the two guards.
- **Verification:** Verified at plugins/storage/service.go:17-23: validatePath only rejects ".." and "../"-prefixed paths after filepath.Clean. An absolute path like "/etc/passwd" cleans to itself and passes the guard, so the in-package validation is weaker than its docstring ("rejects path traversal attempts as defense-in-depth") and weaker than the project's own internal/pathutil/root.go:77 ValidateRelative (which rejects absolute paths, NUL bytes, and "..") used by the sibling plugins/core/storage nodes. The defect is genuine. However, it is NOT exploitable: the local backend (plugin.go:45) wraps OsFs in afero.NewBasePathFs, and I confirmed in the vendored afero@v1.15.0/basepath.go that RealPath does filepath.Clean(filepath.Join(bpath, name)), re-rooting absolute paths under the base and rejecting anything not prefixed by base; the memory backend uses MemMapFs (in-memory only). So this is a real defense-in-depth/consistency gap, not a reachable vulnerability. The finding itself accurately frames it as such. Severity is Low.
- **Status:** Open

### execution-2. Scheduler has no overlap protection; long-running jobs run concurrently and pile up goroutines
- **Files:** `internal/scheduler/runtime.go:108-135`
- **Unit / dimension:** execution / correctness  (reviewer confidence 0.65)
- **Claim:** cron.New(cron.WithSeconds()) is created without a SkipIfStillRunning/DelayIfStillRunning chain wrapper, so if a workflow runs longer than its cron interval the next tick spawns another concurrent execution of the same schedule (duplicate fire for non-idempotent workflows) and goroutines accumulate unboundedly under sustained overload. The per-minute distributed lock does not prevent this because consecutive fires fall in different minute buckets.
- **Evidence:** opts := []cron.Option{cron.WithSeconds()}
	r.cron = cron.New(opts...)
...
	_, err := r.cron.AddFunc(spec, func() {
		r.runJob(sc)
	})
- **Suggested fix:** Wrap jobs with cron.WithChain(cron.SkipIfStillRunning(cronLogger)) (or DelayIfStillRunning) so a schedule cannot overlap itself on a single instance, bounding concurrency to one in-flight run per schedule.
- **Verification:** Confirmed against code and vendored library. In internal/scheduler/runtime.go:108-109 the cron is built only with cron.WithSeconds(); no cron.WithChain(SkipIfStillRunning/DelayIfStillRunning) wrapper is applied. The vendored robfig/cron v3.0.1 (cron.go) confirms New() uses an empty NewChain() and startJob runs every fire in its own `go func()` goroutine, so by default a schedule whose run outlasts its interval overlaps itself concurrently. The mitigations in runJob do not close this: (a) the distributed lock is opt-in (LockEnabled) and the lock key is bucketed per minute (now.Truncate(time.Minute), line 229), so consecutive fires in different minute buckets get different keys and do not block each other — exactly as the finding states; (b) the per-job context timeout (default 5m, lines 205-209) bounds how long each goroutine lives but does not prevent overlap, and only bounds growth if the workflow nodes actually honor ctx cancellation. So duplicate concurrent fires of a non-idempotent workflow are reachable on a single instance, and goroutines accumulate under sustained overload (e.g. a per-second schedule whose job takes minutes). The defect genuinely exists. Severity is moderate-to-low rather than high: each goroutine is time-bounded by the timeout and the per-minute lock can suppress same-minute overlap when enabled, so practical pile-up is bounded; the main correctness risk is duplicate execution of non-idempotent scheduled workflows when locking is off.
- **Status:** Open

### execution-5. Data race: non-atomic read of parent.depth while concurrent loop/workflow.run nodes mutate it atomically
- **Files:** `internal/engine/subworkflow.go:82-84`
- **Unit / dimension:** execution / correctness  (reviewer confidence 0.55)
- **Claim:** runSubWorkflow copies childCtx.depth = parent.depth with a plain field read, but parent.depth is otherwise mutated via atomic.AddInt32 in CheckAndIncrementDepth/DecrementDepth. When two control.loop or workflow.run nodes execute on parallel DAG branches sharing the same parent ExecutionContextImpl, one node's atomic write races with another's plain read — a data race detectable by -race.
- **Evidence:** // Inherit depth from parent
	childCtx.depth = parent.depth
	childCtx.maxDepth = parent.maxDepth   // vs context.go: atomic.AddInt32(&c.depth, 1) / atomic.LoadInt32(&c.maxDepth)
- **Suggested fix:** Read with atomics: childCtx.depth = atomic.LoadInt32(&parent.depth) and atomic.LoadInt32(&parent.maxDepth) (or store via atomic.StoreInt32), keeping all access to the depth fields atomic.
- **Verification:** Confirmed real. subworkflow.go:83 reads parent.depth as a plain field access, while context.go:338/349 mutate the same field with atomic.AddInt32 in CheckAndIncrementDepth/DecrementDepth. Both control/loop.go:99 and workflow/run.go:103 call CheckAndIncrementDepth on the shared parent execCtx, and executor.go:82-100 runs each ready node in its own goroutine over that one shared context. Two workflow.run/control.loop nodes on parallel DAG branches therefore have one goroutine's plain read at line 83 race with another's atomic write to parent.depth — a data race -race would flag. maxDepth (line 84) does NOT race since it is set once at construction and never mutated. Severity lowered to Low: depth is a 4-byte counter so impact is at most a transient stale value / depth-limit miscount, not memory corruption, but it is a genuine data race.
- **Status:** Open

### livekit-1. Unchecked int-to-uint32 conversion on room limits allows silent wraparound
- **Files:** `plugins/livekit/room_create.go:60,66`
- **Unit / dimension:** livekit / correctness  (reviewer confidence 0.6)
- **Claim:** A negative or oversized empty_timeout/max_participants from a resolved expression wraps around instead of being rejected, e.g. max_participants -1 becomes ~4.29 billion (effectively unlimited).
- **Evidence:** req.EmptyTimeout = uint32(v)  ... req.MaxParticipants = uint32(v)  // v is an int from ResolveOptionalInt with no range/sign validation (resolve.go:124 returns int via int(float64) with no bounds check)
- **Suggested fix:** Validate that v >= 0 (and within uint32 range) before conversion, returning an error otherwise, e.g. `if v < 0 || v > math.MaxUint32 { return error }` for both empty_timeout and max_participants.
- **Verification:** Confirmed against the code. plugins/livekit/room_create.go:60 and :66 perform req.EmptyTimeout = uint32(v) and req.MaxParticipants = uint32(v) where v is an int from plugin.ResolveOptionalInt (internal/plugin/resolve.go:124-148). That function returns int(float64)/int with NO sign or range validation, so a negative value silently wraps (uint32(-1) = 4294967295, effectively unlimited). The node's ConfigSchema declares these only as {"type":"integer"} with no minimum, so JSON-schema validation does not reject negatives either. The defect genuinely exists and is reachable through config literals or resolved expressions. However, these values are author/config-controlled in this config-driven runtime rather than attacker-controlled, making this a robustness/input-validation defect rather than a security vulnerability — hence Low severity.
- **Status:** Open

### nodes-api-2. OIDC nodes perform issuer discovery on every execution (no provider caching)
- **Files:** `plugins/core/oidc/auth_url.go:125`
- **Unit / dimension:** nodes-api / correctness  (reviewer confidence 0.45)
- **Claim:** gooidc.NewProvider makes a network round-trip to the issuer's discovery endpoint on every node execution in auth_url, exchange and refresh; on a login hot path this adds latency and makes auth fully dependent on issuer reachability per-request.
- **Evidence:** provider, err := gooidc.NewProvider(ctx, issuerURL)
if err != nil {
    return "", nil, fmt.Errorf("oidc.auth_url: OIDC discovery failed: %w", err)
}
- **Suggested fix:** Cache the *gooidc.Provider keyed by issuerURL (e.g. a package-level sync.Map with a TTL) so discovery is amortized across requests rather than performed on every node execution.
- **Verification:** Verified. plugins/core/oidc/auth_url.go:125 calls gooidc.NewProvider(ctx, issuerURL) inside Execute on every node invocation. The vendored go-oidc source (GOMODCACHE .../coreos/go-oidc@v2.5.0+incompatible/oidc.go) shows NewProvider performs a synchronous http.NewRequest("GET", issuer+"/.well-known/openid-configuration") + doRequest on each call — a real network round-trip with no internal caching. The same uncached pattern is repeated in exchange.go:86 and refresh.go:77. No upstream guard, wrapper, or package-level provider cache exists in the oidc package. So every auth_url/exchange/refresh execution incurs a discovery round-trip, adding login-path latency and coupling each request to issuer reachability. The defect is genuine and reachable; it is a performance/availability concern rather than a security vulnerability, so severity is Low-to-Medium.
- **Status:** Open

### nodes-api-3. email.send does not validate from/reply_to as addresses and silently swallows resolve errors
- **Files:** `plugins/email/send.go:69-70`
- **Unit / dimension:** nodes-api / correctness  (reviewer confidence 0.55)
- **Claim:** from and reply_to are resolved with errors discarded and are never validated with mail.ParseAddress (unlike to/cc/bcc), so a failed expression silently yields an empty/garbage value rather than an error.
- **Evidence:** from, _, _ := plugin.ResolveOptionalString(nCtx, config, "from")
replyTo, _, _ := plugin.ResolveOptionalString(nCtx, config, "reply_to")
- **Suggested fix:** Propagate the resolve errors instead of discarding them, and run mail.ParseAddress on non-empty from/reply_to values for consistency with recipient validation. (Header injection itself is already prevented by sanitizeHeader.)
- **Verification:** Confirmed in plugins/email/send.go:69-70: `from, _, _ := plugin.ResolveOptionalString(...)` and `replyTo, _, _ := ...` discard the error returned by ResolveOptionalString (internal/plugin/resolve.go:34-52, which errors on resolve failure or non-string results). Unlike to/cc/bcc, which are validated via mail.ParseAddress in resolveRecipients (plugins/email/helpers.go:58-65), from/reply_to are never address-validated. A failing/non-string expression therefore silently yields "" — from falls back to the service default (service.go:45-48) and reply_to is omitted — instead of surfacing an error. The defect is real and reachable. It is a correctness/consistency issue only, not a security vuln: header injection is already prevented by sanitizeHeader (service.go:13), as the finding itself notes. Hence Low severity.
- **Status:** Open

### platform-2. Concurrent migration runs are not serialized with a database lock
- **Files:** `internal/migrate/migrate.go:59-99`
- **Unit / dimension:** platform / correctness  (reviewer confidence 0.5)
- **Claim:** Up relies solely on the schema_migrations primary key to prevent double-application; two instances racing both pass the applied[] check, both execute the up SQL, and the loser aborts with a duplicate-key error rather than waiting.
- **Evidence:** `applied, err := getApplied(db)` is read once, then `for _, m := range files { if applied[m.Version] { continue } ... db.Transaction(func(tx){ tx.Exec(string(sql)); tx.Create(&schemaMigration{Version: m.Version}) }) }`. There is no advisory/session lock around the run, so two app replicas (or `migrate up` plus an auto-migrating instance) starting together both see the version as unapplied, both run the DDL, and the second commit fails on the version PK — surfacing as a startup/migration error instead of a clean skip.
- **Suggested fix:** Acquire a database-level lock for the duration of Up (e.g. `SELECT pg_advisory_xact_lock(<const>)` on Postgres, or a dedicated lock row) before reading applied migrations, so concurrent runners serialize and the loser observes the already-applied state instead of a duplicate-key failure.
- **Verification:** Confirmed against internal/migrate/migrate.go:59-99. `getApplied(db)` is read once into a map (line 64), then the loop skips applied versions and otherwise runs the up SQL in a transaction ending with `tx.Create(&schemaMigration{Version})` (lines 86-94). There is no advisory/session lock anywhere in the package — the schema_migrations PK (line 30) is the sole guard, so two concurrent Up runs both see a version as unapplied, both run the DDL, and the loser's tx.Create fails on the version PK, surfacing as a migration error instead of a clean skip. The defect is real. However, severity should be Low: (1) the finding's 'auto-migrating instance' is fictional — migrate.Up is only ever invoked by the explicit `noda migrate up` CLI command (cmd/noda/main.go:608); there is no startup auto-migration, so the race requires an operator deliberately running `migrate up` from two processes at once; (2) on Postgres the transactional DDL rolls the loser's whole transaction back, so there is no double-application or corruption, only a cosmetic duplicate-key error. Real but low-impact robustness gap.
- **Status:** Open

### realtime-2. Trace WebSocket write/read loops have no deadlines or ping; a half-open client leaks goroutines and the connection
- **Files:** `internal/trace/websocket.go:44 (WriteMessage), 52-58 (read loop + writeWg.Wait)`
- **Unit / dimension:** realtime / correctness  (reviewer confidence 0.6)
- **Claim:** The trace WebSocket sets no read or write deadline and sends no pings, so a half-open/stuck client blocks c.WriteMessage indefinitely; close(done) then waits on writeWg.Wait() forever, leaking the handler goroutine, the write goroutine, and the connection (one set per stuck client).
- **Evidence:** Write goroutine: `case msg := <-writeCh: if err := c.WriteMessage(websocket.TextMessage, msg); err != nil { return }` with no SetWriteDeadline anywhere. Read loop: `for { if _, _, err := c.ReadMessage(); err != nil { break } }` with no read deadline. Shutdown path `close(done); writeWg.Wait()` blocks until the write goroutine returns, which never happens if WriteMessage is stuck on an unresponsive client. Unlike connmgr/websocket.go there is no ping ticker / pong-driven read deadline to detect a dead peer.
- **Suggested fix:** Add a write deadline before each WriteMessage (e.g. SetWriteDeadline 5s) and a ping ticker with a pong handler that refreshes a read deadline, matching the lifecycle handling in connmgr/websocket.go, so stuck clients are reaped instead of leaking goroutines.
- **Verification:** Confirmed against /Users/marten/GolandProjects/noda/internal/trace/websocket.go. Line 44 calls c.WriteMessage with no SetWriteDeadline anywhere in the file; the read loop (52-56) uses c.ReadMessage with no read deadline; there is no ping ticker/pong handler. A half-open peer (TCP send buffer full, no FIN/RST) can block WriteMessage indefinitely — the write goroutine then never re-enters its select to observe done, so close(done); writeWg.Wait() (57-58) hangs; and ReadMessage likewise never returns. The handler goroutine, write goroutine, and connection all leak per stuck client. The project's own connmgr/websocket.go (lines 158-171) demonstrates the correct mitigation (SetPongHandler refreshing SetReadDeadline + ping ticker via WriteControl), which this file omits, so the suggested fix is apt. Severity lowered to L because RegisterTraceWebSocket is dev-mode-only (production uses the no-op variant), limiting the impact to local dev/editor sessions rather than a production DoS.
- **Status:** Open

### realtime-3. Dead/misleading WebSocketConfig.MaxPerChannel field is never read
- **Files:** `internal/connmgr/websocket.go:30`
- **Unit / dimension:** realtime / hygiene  (reviewer confidence 0.85)
- **Claim:** WebSocketConfig.MaxPerChannel is never referenced; per-channel limits are only enforced via ManagerConfig.MaxConnectionsPerChannel, so the field is dead and could mislead a maintainer into thinking setting it limits connections.
- **Evidence:** `MaxPerChannel int` is declared in WebSocketConfig but grep shows no read site; enforcement lives in manager.go:83-88 using m.config.MaxConnectionsPerChannel, which connections.go:40-45 populates from channels.max_per_channel — the WebSocketConfig field is never consulted.
- **Suggested fix:** Remove WebSocketConfig.MaxPerChannel (and MaxConcurrentMessages if similarly unset by the server wiring), or actually plumb it into the Manager so the limit is honored, to avoid a silently-ignored limit knob.
- **Verification:** Verified against the code. internal/connmgr/websocket.go:30 declares WebSocketConfig.MaxPerChannel, but grep across all .go files shows no read site in production code — it is only assigned in two tests (coverage_test.go:611,1543). Per-channel limit enforcement is done entirely in manager.go:83-88 using m.config.MaxConnectionsPerChannel, which handleConnection reaches via h.manager.Register(conn) (websocket.go:144). The WebSocketConfig field is never plumbed into the Manager, so setting it has no effect. The integration test TestWebSocketHandler_Integration_MaxPerChannel passes only coincidentally: NewManager() leaves MaxConnectionsPerChannel=0 (unlimited), so the second connection actually registers, then its 1s read times out and it closes, the deferred Unregister drops the count back to 1, and the final Count()==1 assertion passes without ever exercising any MaxPerChannel limit. This confirms the field is dead and misleading. This is a code-quality/maintainability defect, not a security or runtime correctness bug, so severity is Low.
- **Status:** Open

### resolve-1. Unlocked map read of len(r.services) in WithOverrides (latent data race)
- **Files:** `internal/registry/services.go:112-122`
- **Unit / dimension:** resolve / correctness  (reviewer confidence 0.62)
- **Claim:** WithOverrides evaluates len(r.services) as a make() argument before acquiring r.mu.RLock(), an unsynchronized read of the shared services map.
- **Evidence:** func (r *ServiceRegistry) WithOverrides(overrides map[string]any) *ServiceRegistry {
	child := &ServiceRegistry{
		services: make(map[string]serviceEntry, len(r.services)),  // <-- read of r.services without holding r.mu
	}
	r.mu.RLock()
	for name, entry := range r.services {
		child.services[name] = entry
	}
- **Suggested fix:** Acquire r.mu.RLock() first, then compute len(r.services) inside the locked region: take the lock at the top of the function and build the child map (including the make capacity hint) entirely within it.
- **Verification:** Confirmed at internal/registry/services.go:112-122. Line 114 computes `make(map[string]serviceEntry, len(r.services))` before `r.mu.RLock()` on line 116, an unsynchronized read of the shared `r.services` map. This is inconsistent with every other method (Get, All, byPrefix, Count) which read the map only under the lock, and Register mutates it under r.mu.Lock(). A concurrent Register would constitute a data race the Go race detector flags. However, Register runs at startup/init while WithOverrides runs at request time, so a concurrent writer is unlikely in current usage, making it a genuine but latent low-severity defect. The suggested fix (take the lock first, then build the child including the capacity hint inside the locked region) is correct.
- **Status:** Open

### secrets-config-1. Overlay merge security-removal guard watches a non-existent key, leaving global_middleware silently strippable
- **Files:** `internal/config/merge.go:4`
- **Unit / dimension:** secrets-config / security  (reviewer confidence 0.8)
- **Claim:** ValidateMergePreservedKeys guards top-level keys "security" and "middleware", but the real root config has no "middleware" key — the security-bearing key is "global_middleware" — so an overlay that sets global_middleware to null removes all global auth/security middleware without any warning.
- **Evidence:** merge.go:4 `var securityKeys = []string{"security", "middleware"}`. The root schema (internal/config/schemas/root.json) defines `global_middleware`, `middleware_presets`, `middleware_instances` and NO top-level `middleware`. crossrefs.go corsUsed() reads `rc.Root["global_middleware"]` to find security middleware like "security.cors". MergeOverlay (merge.go:33-35) deletes any key whose overlay value is null: `if overlayVal == nil { delete(result, key); continue }`. So a `noda.{env}.json` overlay containing `"global_middleware": null` drops every globally-applied security/auth middleware and ValidateMergePreservedKeys (merge.go:8-19) emits no warning because "global_middleware" is not in securityKeys and "middleware" never matches a base key.
- **Suggested fix:** Replace the dead "middleware" entry with the actual security-relevant root keys: `var securityKeys = []string{"security", "global_middleware", "middleware_presets", "middleware_instances"}`. Consider also warning when an overlay weakens (not only fully removes) the security/global_middleware sections.
- **Verification:** Confirmed against code. merge.go:4 declares securityKeys = {"security","middleware"}. root.json has NO top-level "middleware" key (the security-relevant global key is "global_middleware"; the "middleware" tokens in the schema are nested under route_groups and connection endpoints). MergeOverlay (merge.go:33-35) deletes any key set to null in the overlay, and ValidateMergePreservedKeys (merge.go:8-19) only warns for keys in securityKeys. So an overlay containing "global_middleware": null strips all globally-applied auth/security middleware while emitting no warning, because "global_middleware" isn't in securityKeys and the dead "middleware" entry never matches a base key. crossrefs.go:288/471 confirm global_middleware is the security-bearing array. Reachable: pipeline.go:57-61 calls both functions on real env overlay files. The defect is real. I downgrade severity to Low: this is a defense-in-depth advisory-warning gap (slog.Warn only, not a hard error), the removal requires an operator-authored overlay in the deployment, and the entire "security" section is still guarded.
- **Status:** Open

### wasm-2. No default maximum memory for Wasm modules; MemoryPages=0 leaves guest memory effectively unbounded (OOM risk)
- **Files:** `internal/wasm/runtime.go:73-77 (+ cmd/noda/main.go:910-912, types.go:17)`
- **Unit / dimension:** wasm / security  (reviewer confidence 0.72)
- **Claim:** A WasmManifest memory limit is only applied when cfg.MemoryPages > 0; when the operator omits memory_pages the module can grow to wazero's default ceiling (~64Ki pages / 4GiB), allowing an untrusted module to OOM the host.
- **Evidence:** runtime.go: 'if cfg.MemoryPages > 0 { manifest.Memory = &extism.ManifestMemory{MaxPages: cfg.MemoryPages} }'. parseWasmModuleConfig only sets MemoryPages from config and applies no default (unlike MaxModuleSize/TickTimeout which have defaults). types.go comment: 'MemoryPages uint32 // Max memory pages (0 = default)' but there is no enforced default — wazero leaves the limit at its built-in maximum.
- **Suggested fix:** Apply a conservative default MaxPages (e.g. a few hundred pages) in NewModule/LoadModule when MemoryPages==0, mirroring defaultMaxModuleSize, so memory is bounded by default for untrusted modules.
- **Verification:** Confirmed against actual code and vendored sources. runtime.go:73-77 only sets manifest.Memory.MaxPages when cfg.MemoryPages>0; parseWasmModuleConfig (main.go:910-912) applies no default for memory_pages (unlike MaxModuleSize/TickTimeout which have defaults); types.go:17 comment 'MemoryPages uint32 // Max memory pages (0 = default)' is misleading since no default is enforced. Vendored extism go-sdk@v1.7.1 plugin.go:133-137 calls wazero WithMemoryLimitPages only when MaxPages>0; otherwise wazero@v1.9.0 config.go:198 leaves the default memoryLimitPages = wasm.MemoryLimitPages = 65536 pages (4GiB, confirmed by config_test.go:94 '65537 > 65536'). So omitting memory_pages leaves guest memory bounded only by the ~4GiB WASM ceiling — a real missing-default/hardening defect. Severity reduced to Low: Noda Wasm modules are operator-supplied trusted config artifacts loaded from local paths, not untrusted remote uploads, so the finding's 'untrusted module OOM' framing overstates the threat; it is a defense-in-depth gap, not a remotely exploitable vulnerability.
- **Status:** Open

---
## Repo Hygiene

Mostly clean — the working tree looks cluttered, but the *tracked* repo is in good shape. Items below are deterministic (from Phase-0 git/tooling scans), not agent-derived.

### HYG-1. Root-level scratch docs tracked at repo root (L)
- **Files:** `ISSUES.md`, `FR-expression-string-functions.md`, `FR-parameterized-service-binding.md`, `REVIEW-FINDINGS.md`
- **Claim:** Four working/scratch docs are tracked at the repo root, cluttering the top-level listing. `REVIEW-FINDINGS.md` is the canonical historical tracker (keep), but the `FR-*` feature-request notes and `ISSUES.md` read as transient scratch.
- **Suggested fix:** Move `FR-*.md` / `ISSUES.md` under `docs/_internal/` (or close them into GitHub issues) and keep the root to `README`/`CHANGELOG`/`CLAUDE`/`LICENSE`.
- **Status:** Open

### HYG-2. Eight stale merged-but-unpruned local branches (L)
- **Detail:** `feat/lifecycle-manager`, `feat/livekit`, `feat/oidc`, `feat/production-readyness`, `feat/secrets-manager`, `feat/stream-project`, `feature/proxy-status-remap`, `fix/lifecycle-review-findings` are all merged into `main` but not deleted locally.
- **Suggested fix:** `git branch --merged main | grep -vE '^\*|main' | xargs git branch -d`. (One active worktree branch `feat/screen-share-rooms` is unmerged — leave it.)
- **Status:** Open

### HYG-3. Dependency advisories — uncalled but fixable (L, security)
- **Detail:** `govulncheck` reports 0 vulnerabilities in *called* code, but flags advisories in required modules: `golang.org/x/crypto@v0.51.0` (multiple `ssh` advisories, fixed in v0.52.0) and `github.com/buger/jsonparser@v1.1.1` (DoS GO-2026-4514, fixed in v1.1.2). Noda does not call the vulnerable SSH paths, so risk is currently nil, but bumping is cheap hygiene.
- **Suggested fix:** `go get golang.org/x/crypto@v0.52.0 github.com/buger/jsonparser@v1.1.2 && go mod tidy`, then re-run `govulncheck`.
- **Status:** Open

### HYG-4. Deprecated LiveKit API usage (L)
- **Files:** `plugins/livekit/participant_update.go:92`
- **Detail:** `staticcheck` SA1019 — `perm.Recorder` is deprecated in the LiveKit protocol. Tracked here so a lint upgrade doesn't surprise CI.
- **Suggested fix:** Migrate off `perm.Recorder` per LiveKit's replacement guidance, or annotate with a justified `//nolint`.
- **Status:** Open

### Hygiene — verified clean (no action)
- No tracked binaries, `.wasm`, `dist/`, or `.DS_Store` (the 79 MB `noda` binary, `.DS_Store`, and `.env` exist in the working tree but are all correctly gitignored).
- No secrets ever committed; no hardcoded credentials in non-test `.go`. The working-tree `.env` holds real-looking secrets (`JWT_SECRET`, `DISCORD_BOT_TOKEN`) but is gitignored — keep it that way; never `git add -f`.
- `go.mod` is tidy (`go mod tidy` produces no diff).
- `golangci-lint` (incl. gosec) and `go vet`: **0 issues**.

---

## Appendix A — Rejected during verification

The adversarial verifier refuted these 3 candidate findings. Recorded for transparency; **no action needed**.

### expr-1. Expression evaluation has no CPU/wall-clock timeout or context cancellation; relies solely on the memory budget  (claimed L)
- **Files:** `internal/expr/evaluator.go:51-57`
- **Claim:** runWithBudget executes the expr VM with only a memory budget and no execution-time bound or context cancellation, so any future allocation-light but CPU-heavy expr construct (or a config that raises/disables expression_memory_budget) has no defense-in-depth time limit.
- **Refuted because:** The code description is accurate (runWithBudget at evaluator.go:51-57 applies only a memory budget, no context/deadline), but this is not a reachable defect. (1) The budget cannot be disabled: when c.opts.memoryBudget==0 noda leaves v.MemoryBudget unset, and vendored expr@v1.17.8/vm/vm.go:87-90 then sets it to conf.DefaultMemoryBudget=1e6, so the finding's evidence that expression_memory_budget:0 "disables the only bound" is factually wrong. (2) The memory budget effectively bounds CPU: expr has no language-level loops; iteration occurs only via builtins (map/filter/reduce/range) that push onto the stack and trip memGrow (vm.go:687-691, "memory budget exceeded"). (3) The finding self-concedes it is pure defense-in-depth and that all current DoS vectors are already rejected in microseconds, citing only a hypothetical "future construct." No demonstrated reachable vulnerability exists.

### wasm-4. Unbounded async-call and timer fan-out from a single module enables host resource exhaustion  (claimed M)
- **Files:** `internal/wasm/hostapi.go:104-130 (CallAsync) + internal/wasm/module.go:344-351 (SetTimer)`
- **Claim:** A module can issue an unbounded number of noda_call_async invocations (each spawning a tracked goroutine and a pendingLabels/asyncResults entry) and register unbounded named timers within a single tick, with no per-module cap, allowing CPU/goroutine/memory exhaustion of the host from one untrusted guest.
- **Refuted because:** The code-level facts are accurate: RegisterAsyncLabel (internal/wasm/module.go:376-384) only rejects duplicate labels with no count cap, CallAsync (internal/wasm/hostapi.go:104-130) spawns one tracked goroutine per call with no concurrency ceiling, and SetTimer (internal/wasm/module.go:344-351) grows m.timers unbounded by distinct name while executeTick (internal/wasm/tick.go:60) iterates all timers each tick. BUT the finding mischaracterizes the threat model. In noda, Wasm modules are NOT untrusted multi-tenant guests — they are the operator's own custom logic loaded from project config (CLAUDE.md: "Custom logic runs in Wasm modules"). docs/04-guides/wasm-development.md:461-483 explicitly documents that resource bounding is a deliberate, known limitation: there is no instruction-level metering, only a wall-clock wasmCallTimeout (30s) and per-tick TickTimeout, and the docs state a misbehaving module can already exhaust CPU bounded only by those timeouts ("If a module misbehaves and exhausts CPU, the only process-wide signal is the 30-second timeout"). Thus a module that wanted to DoS the host can already do so via runaway compute — an accepted, documented design tradeoff. The async/timer fan-out adds no new security boundary breach; it is the same already-accepted self-DoS class. Fan-out is also bounded in practice: host-function calls only occur while the guest's tick function executes, which is itself capped by TickTimeout context cancellation. The C3 tracking issue was already fixed. This is at most a marginal defense-in-depth hardening suggestion, not a genuine reachable security vulnerability from an untrusted actor as claimed.

### livekit-2. Ingress URL and egress S3 endpoint forwarded to LiveKit without scheme/host validation  (claimed L)
- **Files:** `plugins/livekit/ingress_create.go:90-94`
- **Claim:** If a developer wires the ingress `url` (URL_INPUT) or egress S3 `endpoint` from request data, the value is passed verbatim to the LiveKit server, which will fetch/connect to it, enabling LiveKit-side SSRF to internal hosts; there is no allowlist or scheme restriction.
- **Refuted because:** Lines 90-94 of plugins/livekit/ingress_create.go do forward the resolved `url` config value verbatim into req.Url with no scheme/host validation, so the code matches the citation. But this is not a concrete reachable vulnerability: `url` is a developer-authored config field (ConfigSchema: "Source URL (required for url input type)"), and Noda's config-driven design treats config values as trusted. The finding is entirely conditional ("if a developer wires the url from request data"), identical to every other URL-accepting node (outbound HTTP, etc.), and the actual outbound fetch occurs on the LiveKit server — a separate component whose documented purpose for URL_INPUT is to ingest from an arbitrary source URL. No noda-side SSRF executes here; nothing in noda connects to the URL. The finding's own fix ("...or document that these fields must never be bound to untrusted input") concedes it is a hardening/documentation recommendation, not a defect. The egress S3 endpoint part of the claim is not even in the cited file.
---

## Appendix B — Tools run / not run

- **Ran:** `golangci-lint run` (incl. gosec), `go vet`, `govulncheck` (go1.25), `staticcheck` (rebuilt on go1.25 via `go run`), scripted secret/history sweep, git hygiene scan.
- **NOT RUN:** `gitleaks` (not installed) — substituted by a scripted `git log -p` / `git grep` credential sweep over history + working tree, which found nothing. Install `gitleaks` for a more exhaustive entropy-based scan if desired.

## Method notes

- 30 candidate findings from 12 domain agents → 27 confirmed after a per-finding adversarial refute-pass that read the actual code and vendored library source (`~/go/pkg/mod`). 42 agents total, ~1.5M tokens.
- Several Mediums cluster around two themes worth a dedicated tranche: **(a) HTTP middleware/auth determinism** (server-1/2/3 — route-group matching + JWT aud/iss/exp), and **(b) goroutine/panic lifecycle** (execution-1/4, wasm-1/3, realtime-1). These mirror the structure of the 2026-04 tranches.

---

## Shipped 2026-06-29 — Tranche: HTTP middleware / auth determinism

Branch `feat/http-auth-determinism`. Closed: **server-1, server-2, server-3**.
- server-1/3: `getGroupMiddleware` now merges all matching route groups (outermost-first, deduped) with segment-aware prefix matching — `internal/server/presets.go`.
- server-2: `auth.jwt` honors optional `audience`/`issuer`/`require_expiry` (default off) — `internal/server/middleware.go`.
- Tests: `internal/server/auth_determinism_test.go` (fail against pre-fix code). Docs: `docs/02-config/middleware.md`.

---

## Shipped 2026-06-29 — Tranche: goroutine / panic lifecycle

Branch `feat/goroutine-lifecycle-hardening`. Closed: **execution-1, execution-4, wasm-3, realtime-1**.
- execution-1: deferred `recover()` inside TimeoutMiddleware's goroutine → panic surfaced as error, not a process crash — `internal/worker/middleware.go`.
- execution-4: top-level `recover()` in `processMessage` (mirrors `scheduler.runJob`) → a pre-handler panic no longer kills the consumer — `internal/worker/runtime.go`.
- wasm-3: `reconnectLoop` now checks the `stopCh` teardown signal pre-sleep, post-sleep, and under-lock post-dial → no socket resurrection after Close — `internal/wasm/gateway.go`.
- realtime-1: bounded per-conn outbound writer goroutine with write deadline (`wsWriter`), mirroring SSE → no broadcast head-of-line blocking — `internal/connmgr/websocket.go`.
- Tests: `internal/worker/middleware_test.go`, `internal/worker/runtime_test.go`, `internal/wasm/gateway_test.go`, `internal/connmgr/websocket_writer_test.go` (all fail against pre-fix code). Docs: `docs/02-config/connections.md`, CHANGELOG.

**wasm-1 deferred** to its own tranche (interruptible Wasm execution — touches Extism/wazero semantics).
