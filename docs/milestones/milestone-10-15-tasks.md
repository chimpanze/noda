# Milestone 10: Cache Plugin — Task Breakdown

**Depends on:** Milestone 3 (plugin system), Milestone 8 (HTTP server)
**Result:** Redis cache operations available in workflows. `CacheService` interface implemented for cross-plugin use.

---

## Task 10.1: Cache Plugin

**Description:** Create the Redis-backed cache plugin.

**Subtasks:**

- [ ] Create `plugins/cache/plugin.go`:
  - Name: `"cache"`, Prefix: `"cache"`
  - HasServices: true
  - CreateService: parse Redis URL, create go-redis client, configure pool
  - HealthCheck: `client.Ping()`
  - Shutdown: close client
- [ ] Implement `api.CacheService` interface on the service instance so other plugins can use it

**Tests:**
- [ ] Plugin registers, creates Redis connection, health checks, shuts down
- [ ] Service implements `CacheService` interface

**Acceptance criteria:** Redis connections managed through plugin lifecycle.

---

## Task 10.2: Cache Node Implementations

**Description:** Implement all `cache.*` nodes.

**Subtasks:**

- [ ] `cache.get`: resolve `key`, call `client.Get()`, return `{ "value": any }`. Missing key → `NotFoundError`.
- [ ] `cache.set`: resolve `key`, `value`, `ttl` (optional, seconds). Call `client.Set()`.
- [ ] `cache.del`: resolve `key`, call `client.Del()`.
- [ ] `cache.exists`: resolve `key`, call `client.Exists()`, return `{ "exists": bool }`.
- [ ] All nodes: ServiceDeps `{ "cache": { prefix: "cache", required: true } }`, pass `context.Context` to Redis calls.
- [ ] Value serialization: store as JSON in Redis, deserialize on read.

**Tests:**
- [ ] Set then get round-trip
- [ ] Get missing key → NotFoundError
- [ ] TTL expiration — set with TTL, wait, get → NotFoundError
- [ ] Del removes key
- [ ] Exists returns true/false correctly
- [ ] Complex values (maps, arrays) serialize and deserialize

**Acceptance criteria:** All cache operations work with Redis.

---

## Task 10.3: End-to-End Tests

**Subtasks:**

- [ ] Test: HTTP request → workflow caches a value → subsequent request reads from cache
- [ ] Test: Cache with TTL → value expires

**Acceptance criteria:** Cache works in HTTP request workflows.

---

---

# Milestone 11: Event System and Workers — Task Breakdown

**Depends on:** Milestone 3 (plugin system), Milestone 4 (workflow engine), Milestone 8 (HTTP server)
**Result:** Events emit to Redis Streams and PubSub. Workers consume from streams and execute workflows. Dead letter queues handle persistent failures. Use Case 2 (SaaS Backend) core features work.

---

## Task 11.1: Stream Plugin

**Description:** Redis Streams wrapper for durable message delivery.

**Subtasks:**

- [ ] Create `plugins/stream/plugin.go`:
  - Name: `"stream"`, Prefix: `"stream"`
  - HasServices: true
  - CreateService: create go-redis client for Streams operations
- [ ] Implement core operations:
  - `Publish(ctx, topic, payload)` — `XADD` to the stream
  - `Subscribe(topic, group, consumer)` — `XREADGROUP` loop
  - `Ack(topic, group, messageID)` — `XACK`
- [ ] Consumer group creation: auto-create group if it doesn't exist (`XGROUP CREATE ... MKSTREAM`)
- [ ] Service also exposes `emit` operation for use by `event.emit` node and Wasm `noda_call`

**Tests:**
- [ ] Publish message to stream
- [ ] Subscribe reads published messages
- [ ] Ack removes message from pending
- [ ] Consumer group auto-creation

**Acceptance criteria:** Redis Streams publish/subscribe/ack works.

---

## Task 11.2: PubSub Plugin

**Description:** Redis PubSub wrapper for real-time fan-out.

**Subtasks:**

- [ ] Create `plugins/pubsub/plugin.go`:
  - Name: `"pubsub"`, Prefix: `"pubsub"`
  - HasServices: true
  - CreateService: create go-redis PubSub client
- [ ] Implement core operations:
  - `Publish(ctx, channel, payload)` — `PUBLISH`
  - `Subscribe(channel, handler)` — `SUBSCRIBE` with message callback
- [ ] Service also exposes `emit` operation for use by `event.emit` node and Wasm `noda_call`

**Tests:**
- [ ] Publish → subscriber receives message
- [ ] Multiple subscribers all receive
- [ ] Unsubscribe stops delivery

**Acceptance criteria:** Redis PubSub publish/subscribe works.

---

## Task 11.3: `event.emit` Node

**Description:** Core node that publishes events to stream or pubsub.

**Subtasks:**

- [ ] Create `plugins/core/event/plugin.go` and `plugins/core/event/emit.go`:
  - Prefix: `"event"`, Node: `event.emit`
  - ServiceDeps: `stream` (optional), `pubsub` (optional) — required based on `mode`
  - ConfigSchema: `mode` (static: `"stream"|"pubsub"`), `topic` (expression), `payload` (expression)
- [ ] Execute: resolve `topic` and `payload`, call the appropriate service's emit operation
- [ ] Startup validation: verify the slot matching `mode` is filled

**Tests:**
- [ ] Stream mode emits to stream service
- [ ] PubSub mode emits to pubsub service
- [ ] Missing slot for configured mode → startup error
- [ ] Topic and payload expressions resolve correctly

**Acceptance criteria:** Events publish to the correct service based on mode.

---

## Task 11.4: Worker Runtime

**Description:** Consume messages from Redis Streams and execute workflows.

**Subtasks:**

- [ ] Create `internal/worker/runtime.go`
- [ ] Implement `WorkerRuntime`:
  - Load worker configs
  - For each worker: create a consumer that reads from the configured stream topic and consumer group
  - On message: run trigger mapping (message payload → `$.input`), execute the linked workflow
  - Concurrency: configurable number of concurrent message processors per worker
  - On workflow success: ack the message
  - On workflow failure: nack (message returns to pending for redelivery)
- [ ] `$.trigger` metadata: `{ type: "event", timestamp, trace_id, topic, group }`
- [ ] Graceful shutdown: stop consuming, wait for in-flight workflows to complete

**Tests:**
- [ ] Worker consumes messages and executes workflows
- [ ] Trigger mapping populates `$.input` from message payload
- [ ] Concurrency: multiple messages processed concurrently
- [ ] Success → message acked
- [ ] Failure → message redelivered

**Acceptance criteria:** Workers consume and process events reliably.

---

## Task 11.5: Worker Middleware

**Description:** Middleware system for worker message processing (separate from HTTP middleware).

**Subtasks:**

- [ ] Create `internal/worker/middleware.go`
- [ ] Implement worker middleware wrapping message processing:
  - `worker.log` — log message received, processing time, success/failure
  - `worker.timeout` — enforce timeout on workflow execution via context deadline
  - `worker.recover` — catch panics during workflow execution
- [ ] Middleware chain applied per worker from config
- [ ] Same config pattern as HTTP middleware (array of middleware names) but separate implementation

**Tests:**
- [ ] Logging middleware produces structured logs with message metadata
- [ ] Timeout middleware cancels workflow after deadline
- [ ] Recovery middleware catches panics

**Acceptance criteria:** Worker middleware applies cross-cutting concerns.

---

## Task 11.6: Dead Letter Queue

**Description:** Move persistently failing messages to a dead letter topic.

**Subtasks:**

- [ ] Track delivery attempts per message (using Redis Stream's pending entry info or a counter)
- [ ] When a message has been attempted `dead_letter.after` times:
  - Publish the original message to the dead letter topic (configured per worker)
  - Ack the original message (remove from main stream)
  - Log the dead letter event with trace ID and error details
- [ ] Worker error mapping: log errors with full context (trace_id, node_id, error details)

**Tests:**
- [ ] Message fails N times → moves to dead letter topic
- [ ] Original message acked after dead letter
- [ ] Dead letter message contains original payload and error info
- [ ] Error logging includes trace ID

**Acceptance criteria:** Persistently failing messages are safely moved to dead letter.

---

## Task 11.7: End-to-End Tests

**Subtasks:**

- [ ] Test: HTTP request → workflow emits event → worker consumes → worker workflow executes → side effect observable
- [ ] Test: Worker failure → retry → dead letter
- [ ] Test: Multiple workers consuming same topic with consumer group (no duplicate processing)
- [ ] Write `noda test` files for worker workflows

**Acceptance criteria:** Full async event processing pipeline works.

---

---

# Milestone 12: Scheduler Runtime — Task Breakdown

**Depends on:** Milestone 10 (cache plugin for distributed locking)
**Result:** Cron jobs fire on schedule with distributed locking preventing duplicate execution.

---

## Task 12.1: Scheduler Runtime

**Description:** Cron-based scheduler that triggers workflows on schedule.

**Subtasks:**

- [ ] Create `internal/scheduler/runtime.go`
- [ ] Implement `SchedulerRuntime`:
  - Load schedule configs
  - Register cron jobs via `robfig/cron/v3` with `cron.WithSeconds()` for sub-minute precision
  - Each job: acquire lock → run trigger mapping → execute workflow → release lock
  - Timezone per job via `cron.WithLocation()`
  - `$.trigger` metadata: `{ type: "schedule", timestamp, trace_id, schedule_id, cron }`
- [ ] Scheduler error mapping: log failures with trace ID and schedule metadata. Record in job execution history (in-memory for now).

**Tests:**
- [ ] Job fires at configured time (use short intervals for testing)
- [ ] Timezone configuration works
- [ ] Trigger mapping populates `$.input`
- [ ] Error logging includes schedule context

**Acceptance criteria:** Cron jobs fire and execute workflows on schedule.

---

## Task 12.2: Distributed Locking

**Description:** Prevent duplicate execution across multiple Noda instances.

**Subtasks:**

- [ ] Implement lock via cache service: atomic `SET NX` with TTL
- [ ] Lock key: `noda:schedule:{schedule_id}:{execution_time}`
- [ ] If lock acquired → execute workflow, release lock on completion
- [ ] If lock not acquired → skip (another instance is handling it)
- [ ] Lock TTL: from schedule config `lock.ttl`, prevents stuck locks if instance crashes
- [ ] Release lock after workflow completes (success or failure)

**Tests:**
- [ ] Single instance acquires lock and executes
- [ ] Second instance fails to acquire lock and skips
- [ ] Lock expires after TTL if instance crashes
- [ ] Lock released after workflow completes

**Acceptance criteria:** Only one instance executes each scheduled job.

---

## Task 12.3: CLI and End-to-End Tests

**Subtasks:**

- [ ] `noda migrate` equivalent for schedules: not needed (schedules are config)
- [ ] Add schedule status display to CLI (list schedules, last run, next run)
- [ ] Test: scheduled workflow executes and produces observable side effect
- [ ] Test: two instances running → only one executes the job

**Acceptance criteria:** Scheduler works with distributed locking.

---

---

# Milestone 13: Storage and Upload — Task Breakdown

**Depends on:** Milestone 3 (plugin system), Milestone 8 (HTTP server)
**Result:** File storage operations work with local filesystem and S3. File uploads stream to storage with validation.

---

## Task 13.1: Storage Plugin

**Description:** Afero-based storage plugin with local and S3 backends.

**Subtasks:**

- [ ] Create `plugins/storage/plugin.go`:
  - Name: `"storage"`, Prefix: `"storage"`
  - HasServices: true
  - CreateService: based on `backend` config (`"local"`, `"s3"`, `"memory"`), create appropriate Afero filesystem
  - Implement `api.StorageService` interface on the service
- [ ] Local backend: `afero.NewBasePathFs(afero.NewOsFs(), config.path)`
- [ ] S3 backend: Afero S3 adapter with bucket, region, credentials from config
- [ ] Memory backend: `afero.NewMemMapFs()` (for testing)
- [ ] Multiple named instances supported

**Tests:**
- [ ] Local: write, read, list, delete files
- [ ] Memory: same operations (used in tests)
- [ ] Multiple instances with different backends
- [ ] Service implements `StorageService` interface

**Acceptance criteria:** Storage abstraction works across backends.

---

## Task 13.2: Storage Node Implementations

**Subtasks:**

- [ ] `storage.read`: resolve `path`, read via Afero, return `{ "data", "size", "content_type" }`. Missing file → `NotFoundError`.
- [ ] `storage.write`: resolve `path`, `data`, optional `content_type`, write via Afero.
- [ ] `storage.delete`: resolve `path`, delete via Afero.
- [ ] `storage.list`: resolve `prefix`, list via Afero, return `{ "paths": [] }`.
- [ ] All nodes: ServiceDeps `{ "storage": { prefix: "storage", required: true } }`.

**Tests:**
- [ ] Write → read round-trip
- [ ] Read missing file → NotFoundError
- [ ] Delete → read → NotFoundError
- [ ] List returns correct paths with prefix filter

**Acceptance criteria:** All storage operations work through the plugin.

---

## Task 13.3: `upload.handle` Node

**Description:** Process multipart file uploads with validation and streaming.

**Subtasks:**

- [ ] Create `plugins/core/upload/plugin.go` and `plugins/core/upload/handle.go`:
  - Prefix: `"upload"`, Node: `upload.handle`
  - ServiceDeps: `{ "destination": { prefix: "storage", required: true } }`
  - ConfigSchema: `max_size` (static size), `allowed_types` (static string array), `max_files` (static int), `path` (expression)
- [ ] Execute:
  - Receive file stream from trigger mapping (via `files` array)
  - Check Content-Length against `max_size` before fully reading
  - Check MIME type against `allowed_types`
  - Resolve `path` expression for storage destination
  - Stream file directly to storage service (no full buffering)
  - Return `{ "path", "size", "content_type", "filename" }`
  - Validation failure → `ValidationError`

**Tests:**
- [ ] Valid file uploaded and stored
- [ ] Oversized file rejected before fully reading
- [ ] Wrong MIME type rejected
- [ ] Multiple files (up to `max_files`)
- [ ] Exceeding `max_files` → error
- [ ] Path expression resolves correctly (e.g., `avatars/{{ auth.sub }}/{{ $uuid() }}`)

**Acceptance criteria:** File uploads stream to storage with validation.

---

## Task 13.4: End-to-End Tests

**Subtasks:**

- [ ] Test: HTTP file upload → upload.handle → storage.read verifies file exists
- [ ] Test: Two storage instances in one workflow (upload to one, copy to another)
- [ ] Test: Upload validation rejection returns 422

**Acceptance criteria:** File handling works end-to-end.

---

---

# Milestone 14: Image Processing — Task Breakdown

**Depends on:** Milestone 13 (storage plugin)
**Result:** Image manipulation nodes work with bimg/libvips, reading from and writing to storage services.

---

## Task 14.1: Image Plugin

**Description:** bimg-based image processing plugin.

**Subtasks:**

- [ ] Create `plugins/image/plugin.go`:
  - Name: `"image"`, Prefix: `"image"`
  - HasServices: false (uses storage services via slots)
  - Nodes: `image.resize`, `image.crop`, `image.watermark`, `image.convert`, `image.thumbnail`
- [ ] All nodes share ServiceDeps: `{ "source": { prefix: "storage" }, "target": { prefix: "storage" } }`
- [ ] Common pattern: read from source storage → process with bimg → write to target storage

**Acceptance criteria:** Image plugin registered with all nodes.

---

## Task 14.2: Image Node Implementations

**Subtasks:**

- [ ] `image.resize`: read `input` path from source, resize to `width`×`height`, write to `output` path on target. Options: `quality`, `format`.
- [ ] `image.crop`: similar, with `gravity` option (`center`, `smart`, directional).
- [ ] `image.watermark`: read source image and watermark image, composite with `opacity` and `position`.
- [ ] `image.convert`: format conversion (`jpeg`, `png`, `webp`, `avif`) with quality.
- [ ] `image.thumbnail`: fixed dimensions with smart crop, always crops to exact size.
- [ ] All nodes return `{ "path", "width", "height", "size" }`.

**Tests:**
- [ ] Resize produces correct dimensions (verify output metadata)
- [ ] Crop with smart gravity
- [ ] Format conversion (PNG → WEBP, verify output format)
- [ ] Thumbnail exact dimensions
- [ ] Quality setting affects output size
- [ ] Source and target can be different storage instances

**Acceptance criteria:** All image operations produce correct results.

---

## Task 14.3: End-to-End Tests

**Subtasks:**

- [ ] Test: upload image → resize → store thumbnail → read thumbnail via API
- [ ] Test: convert PNG upload to WEBP

**Acceptance criteria:** Image pipeline works with storage.

---

---

# Milestone 15: HTTP Client and Email — Task Breakdown

**Depends on:** Milestone 3 (plugin system), Milestone 8 (HTTP server)
**Result:** Outbound HTTP requests and email sending available in workflows.

---

## Task 15.1: HTTP Client Plugin

**Description:** Outbound HTTP request plugin.

**Subtasks:**

- [ ] Create `plugins/http/plugin.go`:
  - Name: `"http"`, Prefix: `"http"`
  - HasServices: true
  - CreateService: create `net/http.Client` with configured default timeout, optional base URL, default headers
- [ ] Nodes: `http.request`, `http.get`, `http.post`
- [ ] `http.request`: resolve `method`, `url`, `headers`, `body`, `timeout`. Execute request, return `{ "status", "headers", "body" }`.
- [ ] `http.get`: shorthand — method fixed to GET, no body.
- [ ] `http.post`: shorthand — method fixed to POST.
- [ ] Timeout: per-request timeout via context, falls back to service default.
- [ ] Response body: auto-detect JSON and parse, otherwise return as string.

**Tests:**
- [ ] GET request to mock server → correct response
- [ ] POST with JSON body → correct request shape
- [ ] Custom headers sent
- [ ] Timeout → `TimeoutError`
- [ ] Non-200 response → still success output (status code in response data)
- [ ] Connection error → node error

**Acceptance criteria:** Outbound HTTP requests work with all methods.

---

## Task 15.2: Email Plugin

**Description:** SMTP email sending plugin.

**Subtasks:**

- [ ] Create `plugins/email/plugin.go`:
  - Name: `"email"`, Prefix: `"email"`
  - HasServices: true
  - CreateService: configure SMTP connection (host, port, username, password, TLS)
- [ ] `email.send`: resolve `to`, `subject`, `body`, `from`, `cc`, `bcc`, `reply_to`. Static `content_type` (`"text"|"html"`).
- [ ] Return `{ "message_id" }`.
- [ ] Support `to` as string or array of strings.

**Tests:**
- [ ] Send email to mock SMTP server (MailHog or similar)
- [ ] HTML content type
- [ ] Multiple recipients (to, cc, bcc)
- [ ] SMTP error → node error

**Acceptance criteria:** Emails send via SMTP.

---

## Task 15.3: End-to-End Tests

**Subtasks:**

- [ ] Test: webhook → workflow makes outbound HTTP call → uses response in subsequent node
- [ ] Test: workflow sends email after processing
- [ ] Test: HTTP timeout handling in workflow error path

**Acceptance criteria:** Outbound integrations work in workflows.
