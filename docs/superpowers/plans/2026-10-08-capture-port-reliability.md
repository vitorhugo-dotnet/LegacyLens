# Capture Port Reliability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the unacknowledged one-shot content→background event send with a bounded, correlated transport that prefers `chrome.runtime.Port` and falls back to acknowledged `runtime.sendMessage` when the Port receiver does not complete its handshake.

**Architecture:** The content script owns one Port attempt for an active capture and holds unacknowledged events in a FIFO capped at 100. It waits 5 seconds for the session handshake. If no `ready` arrives, it performs the same sender/session handshake through `runtime.sendMessage`; both paths pass events through the same background validation and `CaptureController`, and receive ACK only after `trace.ingest` succeeds. Retryable delivery failures use delays of 250, 1,000, and 2,000 ms, replaying the same event IDs; core deduplication makes replay idempotent. This fallback is based on Windows CI evidence: `onConnect` was registered but never called for the content Port.

**Tech Stack:** TypeScript, Chrome MV3 `runtime.Port`, WXT, Vitest, Playwright Windows E2E, existing `CaptureController` and native core.

**Spec:** `docs/superpowers/specs/2026-10-08-capture-event-transport-diagnostics-design.md`

## Global Constraints

- Keep origin, sender tab, active-session, expiry, event-kind, metadata, and identity validation in the background.
- Use Port name `legacylens.capture.v1` and protocol version `1`.
- Keep at most 100 pending content events per capture; send one event at a time to preserve order.
- ACK only after the host accepts `trace.ingest`; duplicate acceptance is successful delivery.
- Preserve `eventId` across every reconnect/retry. Core identity is `(projectId, traceId, producerId, eventId)`.
- Retry only retryable disconnect/timeout failures, at most three connection attempts. Permanent NACKs fail the queued request.
- If the queue is full, reject the new event and persist an `extension.gap` marker when the Port is available; never silently evict an older event.
- Stop reconnecting on `capture.end`, tab close, expired session, or sender-origin mismatch. Page navigation must not let an old session reconnect from a different origin.
- Keep fixture diagnostics and the fixture capture driver unavailable in production builds.
- Register Port and message transport listeners before optional context-menu initialization.
- Never accept a message fallback without validating sender tab, HTTP(S) origin, active session, and expiry; never ACK before host persistence.
- Do not change the native host protocol or core deduplication key.

## Review Focus

- A background restart after host persistence but before ACK must replay the same `eventId` and leave exactly one visible event; cover in the Port-client and E2E tasks.
- Invalid sender tab/origin or stale session must receive a permanent NACK and never reach `CaptureController.record`; cover in Task 2.
- Disconnect during an in-flight event must retain ordering, respect the 100-event cap, and surface terminal failure after three reconnect attempts; cover in Task 3.
- Stop, tab close, and navigation to a different origin must close the Port and discard no event silently; cover in Tasks 2–3.
- A permanent native rejection must not be retried; cover in Tasks 2–3.

---

## Protocol

- `hello`: `{ type: 'hello', version: 1, traceId: string }`
- `ready`: `{ type: 'ready', version: 1, traceId: string }`
- `event`: `{ type: 'event', version: 1, traceId: string, eventId: string, kind: string, parentEventId?: string, metadata: Record<string, string> }`
- `ack`: `{ type: 'ack', version: 1, traceId: string, eventId: string, gap: boolean, duplicate: boolean }`
- `nack`: `{ type: 'nack', version: 1, traceId: string, eventId?: string, code: string, retryable: boolean }`
- `overflow`: `{ type: 'overflow', version: 1, traceId: string, eventId: string }`; background records `extension.gap` using that stable ID and replies with a matching `ack`.

Protocol parsers must reject unknown versions, malformed IDs, unsupported message types, oversized metadata, and unknown fields that would otherwise be forwarded to the host. Safe NACK codes are fixed constants; page values and raw host errors never become diagnostic codes.

### Task 1: Define and test the Port protocol

**Files:**

- Create: `apps/extension/src/capture/port-protocol.ts`
- Create: `apps/extension/src/capture/port-protocol.test.ts`

**Interfaces:**

- Produce discriminated unions `CapturePortClientMessage` and `CapturePortServerMessage` from the Protocol section.
- Produce `parseCapturePortClientMessage(value: unknown): CapturePortClientMessage | undefined` and the corresponding server parser.
- Produce `CAPTURE_PORT_NAME`, `CAPTURE_PORT_VERSION`, `MAX_PENDING_CAPTURE_EVENTS`, and fixed reconnect constants matching Global Constraints.

- [ ] **Step 1: Write parser tests.** Cover each valid message plus wrong version, invalid trace/event IDs, unknown type, malformed metadata, extra fields, and unsupported NACK codes.
- [ ] **Step 2: Run the protocol tests and confirm they fail.**

Run: `npm run test --workspace apps/extension -- src/capture/port-protocol.test.ts`

Expected: FAIL because the Port protocol module is absent.

- [ ] **Step 3: Implement the exact message unions, constants, and parsers.** Keep metadata validation aligned with the current background allowlist; do not widen `CaptureEventRequest`.
- [ ] **Step 4: Run the focused protocol tests and extension typecheck.**

Run: `npm run test --workspace apps/extension -- src/capture/port-protocol.test.ts`

Run: `npm run typecheck --workspace apps/extension`

Expected: PASS; malformed envelopes return `undefined` and valid envelopes retain their typed identity.

- [ ] **Step 5: Commit the protocol.**

### Task 2: Validate Port connections and ACK after host ingestion

**Files:**

- Create: `apps/extension/src/capture/background-port.ts`
- Create: `apps/extension/src/capture/background-port.test.ts`
- Modify: `apps/extension/entrypoints/background.ts`
- Reuse: `apps/extension/src/capture/background-messages.ts`
- Modify: `apps/extension/src/capture/session.ts` and `apps/extension/src/capture/session.test.ts`

**Interfaces:**

- `handleCapturePort(port: chrome.runtime.Port, controller: CaptureEventController, diagnostics: CaptureBoundaryDiagnostics): () => void` validates the connection, processes messages serially, and returns a cleanup function for stop/tab/origin lifecycle.
- The accepted event message is adapted to the existing `handleCaptureEvent` boundary; ACK follows only after its `{ accepted: true }` reply.
- `CaptureController.record(...)` returns `{ gap: boolean; duplicate: boolean }`; `duplicate` comes from `trace.ingest`, and a duplicate result remains an accepted ACK.
- `chrome.runtime.onConnect` accepts only the Port name from Task 1 and HTTP(S) content-script senders with `sender.tab.id`.
- Register `runtime.onMessage` as an early fallback for `capture.ready` and `capture.event`; reuse the same sender/session/origin validation and event recorder as the Port path.

- [ ] **Step 1: Extend controller tests for host identity.** Change `CaptureController.record(...)` to return `{ gap: boolean; duplicate: boolean }`, preserving per-tab serialization and existing timeout/rejection behavior. Add tests for accepted and duplicate `trace.ingest` results.
- [ ] **Step 2: Write background Port tests.** Cover valid hello/ready, event ACK after record resolves, retryable host timeout NACK, permanent validation NACK, missing tab, non-HTTP sender, origin mismatch, expired/missing session, malformed messages, duplicate event ACK, stop cleanup, and disconnect cleanup. Assert rejected cases never call `controller.record`.
- [ ] **Step 3: Run the controller and background Port tests and confirm the new Port tests fail.**

Run: `npm run test --workspace apps/extension -- src/capture/background-port.test.ts`

Expected: FAIL because `handleCapturePort` is not implemented.

- [ ] **Step 4: Implement sender binding, hello/session handshake, serial message handling, ACK/NACK mapping, and cleanup.** NACK codes are fixed; only disconnect and typed native timeout are retryable. For `overflow`, record `extension.gap` before ACK.
- [ ] **Step 5: Register `onConnect` and the acknowledged message fallback at the beginning of background startup.** Reject unknown Port names. Only enable message fallback after the Port handshake times out; both transports must call the same validation/recording functions.
- [ ] **Step 6: Drain the Port before user stop.** Make `capture.end` wait for content's `CapturePortClient.stop()` to drain/close; then call `CaptureController.stop`. On tab removal or top-level navigation, mark the trace incomplete and stop it without opening the investigation page.
- [ ] **Step 7: Run focused controller and background Port tests plus typecheck.**

Run: `npm run test --workspace apps/extension -- src/capture/session.test.ts src/capture/background-port.test.ts`

Run: `npm run typecheck --workspace apps/extension`

Expected: PASS; only the active tab/origin/session can deliver events, and ACK waits for host completion.

- [ ] **Step 8: Commit background Port handling.**

### Task 3: Add the bounded reconnecting content Port client

**Files:**

- Create: `apps/extension/src/capture/port-client.ts`
- Create: `apps/extension/src/capture/port-client.test.ts`
- Modify: `apps/extension/entrypoints/content.ts`

**Interfaces:**

- `CapturePortClient` accepts injected `connect(name)`, message fallback, timers, and safe diagnostics for unit tests; production injects Chrome runtime APIs.
- `start(traceId): Promise<void>` opens a Port and resolves only after `ready` for that trace.
- `send(event): Promise<{ accepted: true; gap: boolean; duplicate: boolean }>` enqueues FIFO, returns on matching ACK, and rejects on permanent NACK, stop, queue overflow, or exhausted reconnects.
- `stop(code: string): Promise<void>` drains acknowledged work for at most 35 seconds, then disconnects, rejects any remaining requests, clears timers, and disables reconnection.

- [ ] **Step 1: Write client tests.** Cover handshake, one-at-a-time FIFO, matching ACK, unrelated ACK, 100-event capacity, overflow marker, retryable NACK, permanent NACK, disconnect/reconnect with the same event ID, exact backoff and attempt cap, ACK timeout, and stop during reconnect.
- [ ] **Step 2: Run the client tests and confirm they fail.**

Run: `npm run test --workspace apps/extension -- src/capture/port-client.test.ts`

Expected: FAIL because `CapturePortClient` is not implemented.

- [ ] **Step 3: Implement the FIFO and handshake.** Keep one in-flight event; remove it only on matching ACK or permanent failure. Preserve the original event ID and payload across retries.
- [ ] **Step 4: Implement bounded reconnect behavior.** Retry disconnect/timeout failures at 250 ms, 1,000 ms, and 2,000 ms, up to three attempts; keep pending events during retry and reject all when stopped or exhausted.
- [ ] **Step 5: Replace the current message-helper wiring in `content.ts`.** Start the Port after validating `capture.begin`; after a 5-second handshake timeout, negotiate and send through the acknowledged message fallback. Preserve the same FIFO, event IDs, retry bounds, and cleanup in both modes.
- [ ] **Step 6: Run client, controller, and extension tests plus typecheck.**

Run: `npm run test --workspace apps/extension -- src/capture/port-client.test.ts`

Run: `npm run test --workspace apps/extension`

Run: `npm run typecheck --workspace apps/extension`

Expected: PASS; content resolves only on a matching host-backed ACK, and disconnects never silently discard queued events.

- [ ] **Step 7: Commit the content Port client.**

### Task 4: Pin idempotent replay and lifecycle behavior in CI

**Files:**

- Modify: `apps/extension/src/capture/background-port.test.ts` and `apps/extension/src/capture/port-client.test.ts`
- Modify: `tests/e2e/legacy-click.spec.ts`
- Modify: `tests/e2e/modern-click.spec.ts`
- Modify: `tests/e2e/support/capture.ts` only if a fixture Port disconnect control is required
- Verify existing: `core/internal/adapters/sqlite/traces_test.go`

**Interfaces:**

- E2E diagnostic records correlate the exact selected `eventId` through content selection, Port connect/reconnect, background validation, host ingest, ACK, and `investigation.get`.
- Existing dedup key is unchanged. Replayed click has `duplicate: true` ACK after core reports duplicate acceptance.

- [ ] **Step 1: Extend the existing `TestTraceCapturePersistsLifecycleEventsAndDiagnostics` test.** After the duplicate append, load the trace and assert it contains exactly one copy of the identity `(projectID, traceID, producerID, eventID)`.
- [ ] **Step 2: Add client/background lifecycle tests for a worker disconnect after host persistence but before ACK.** Reconnect and replay the same event ID; assert one stored event, an ACK, and FIFO order for subsequent queued events.
- [ ] **Step 3: Update both fixture E2Es to require `jsf.click` in the expected trace with the exact selected event ID and a completed ACK stage.** Include safe Port stages on failure.
- [ ] **Step 4: Add an E2E or background lifecycle case for stop, tab close, and navigation to a different origin.** Assert no stale Port accepts a later event and pending requests terminate visibly.
- [ ] **Step 5: Run local verification.**

Run: `npm run typecheck --workspace apps/extension`

Run: `npm run test --workspace apps/extension`

Run: `go test ./internal/adapters/sqlite -run TestTraceCapturePersistsLifecycleEventsAndDiagnostics -count=1` from `core/`

Run: `npm run test --workspace tests/e2e -- --list`

Run: `LEGACYLENS_FIXTURE_EXTENSION=1 npm run build --workspace apps/extension`

Expected: all commands pass and all five E2E tests are discovered.

- [ ] **Step 6: Commit the reliability and E2E assertions, push `main`, and inspect the Windows CI artifacts.** Required checks and both click-flow fixtures must pass; a repeated event ID must appear once in the expected trace.

## Self-review

- **CI-driven transport adaptation:** runs `37823952379`, `37825808433`, and `37826551410` showed the Port handshake times out even when the background listener is registered. The fallback keeps the approved validation/ACK/idempotency design and changes only the content→background carrier after failed negotiation.

- **Spec coverage:** Port handshake, active-session/origin checks, ACK after native acceptance, idempotent retry, bounded queue/backoff, terminal NACKs, fixture-only diagnostics, and E2E identity correlation each map to Tasks 1–4.
- **Step clarity:** Every task names its module, API, focused test, and expected outcome. No task implements retry without the core deduplication check.
- **Type consistency:** Both directions use versioned discriminated envelopes; Task 2 consumes Task 1 parsers, and Task 3 consumes the same message unions and reconnect constants.
- **Review focus:** Sender mismatch, lost ACK, duplicate delivery, retryable/permanent failure, queue saturation, stop, tab removal, navigation, and restart all have named tests.
- **Proportion:** The plan is limited to content↔background delivery. Native protocol, investigation API, menu UI, and page-world instrumentation stay unchanged.
