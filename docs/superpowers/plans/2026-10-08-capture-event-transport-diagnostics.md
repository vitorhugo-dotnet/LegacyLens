# Capture Event Transport Diagnostics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Instrument the `jsf.click` path, identify and correct the boundary that drops the event, and prove it reaches the expected trace before considering a transport redesign.

**Architecture:** Keep the current one-shot `chrome.runtime.sendMessage` path during diagnosis. Add fixture-only, privacy-safe stage records with `traceId`, `tabId`, `eventId`, outcome, and duration; require an explicit acknowledgement from the background; correlate that acknowledgement with the `trace.ingest` result and the E2E investigation query. Do not implement `chrome.runtime.Port` in this plan; if CI proves the one-shot transport itself is the failing boundary, update the design and prepare a focused follow-up plan.

**Tech Stack:** TypeScript, Chrome MV3 runtime messaging, WXT, Vitest, Playwright Windows E2E, existing `CaptureController` and `NativeClient`.

**Spec:** `docs/superpowers/specs/2026-10-08-capture-event-transport-diagnostics-design.md`

## Global Constraints

- Instrument the current one-shot transport before deciding whether to use `chrome.runtime.Port`.
- Correlate a selected click using the same `traceId`, `tabId`, and `eventId` at each stage.
- Do not log HTML, input values, request contents, tokens, cookies, or full event metadata.
- Restrict detailed diagnostic records to fixture/test builds; production builds must not expose fixture globals or verbose diagnostics.
- A content script must treat a missing or malformed response as an unacknowledged event, not as success.
- Keep sender origin and active capture validation in the background.
- Do not retry an event in this phase; use its stable `eventId` for diagnosis and avoid changing delivery semantics before the failure boundary is known.
- The authoritative browser acceptance run is the GitHub Actions Windows E2E workflow.

## Review Focus

- A sender with no tab ID or HTTP(S) URL gets a bounded, explicit result for a `capture.event` request; test this in Task 2.
- A `sendMessage` call resolving `undefined` or with an incomplete reply is reported as unacknowledged; test this in Task 1.
- A mismatched, expired, or absent capture session is rejected without sending an event to the host; test these cases in Task 2.
- Native host rejection and timeout are distinguishable from a valid `trace.ingest` response; test success and failure in Task 3.
- An event in a different trace or with a different `eventId` does not satisfy the E2E assertion; pin identity correlation in Task 4.

---

### Task 1: Define safe stage diagnostics and an explicit content acknowledgement

**Files:**
- Create: `apps/extension/src/capture/diagnostics.ts`
- Create: `apps/extension/src/capture/diagnostics.test.ts`
- Create: `apps/extension/src/capture/messages.ts`
- Create: `apps/extension/src/capture/messages.test.ts`
- Modify: `apps/extension/entrypoints/content.ts`

**Interfaces:**
- `CaptureStageRecord`: stage, outcome, `traceId`, `tabId`, optional `eventId`, `durationMs`, and safe error code. It must not contain the event metadata or arbitrary error text.
- `recordCaptureStage(record, manifest, sink)`: records only when the fixture manifest contains `http://127.0.0.1/*`; the sink defaults to structured extension logging and is injectable in tests.
- `CaptureEventRequest`: the existing `capture.event` message shape with `sessionId`, `kind`, optional `eventId`/`parentEventId`, and metadata.
- `requestCaptureEvent(sendMessage, message, diagnostics) -> Promise<{ accepted: true; gap: boolean }>`: records `content.send.started` and `content.send.acknowledged`; throws a typed transport error for `undefined`, malformed acknowledgement, explicit error, promise rejection, or a 35,000 ms acknowledgement timeout.

- [x] **Step 1: Add diagnostics tests.** Verify fixture manifests emit only allowlisted fields, production manifests emit nothing, and arbitrary metadata/error text is never copied into a record.
- [x] **Step 2: Run the diagnostics tests and confirm they fail.**

Run: `npm run test --workspace apps/extension -- src/capture/diagnostics.test.ts`

Expected: FAIL because `recordCaptureStage` is not implemented.

- [x] **Step 3: Implement the fixture-gated stage recorder.** Use a small fixed stage/outcome vocabulary; never serialize the full request or error object.
- [x] **Step 4: Add acknowledgement tests.** Cover `{ accepted: true, gap: false }`, `{ error: ... }`, `undefined`, malformed objects, and a rejected send promise. Assert that only the explicit accepted response resolves successfully.
- [x] **Step 5: Run the messaging tests and confirm they fail.**

Run: `npm run test --workspace apps/extension -- src/capture/messages.test.ts`

Expected: FAIL because the typed request helper is not implemented.

- [x] **Step 6: Implement the message helper and use it for every `capture.event` send in `content.ts`.** Pass correlation fields to diagnostics, and dispatch no diagnostic event into the page's main world.
- [x] **Step 7: Run both focused tests and extension typecheck.**

Run: `npm run test --workspace apps/extension -- src/capture/diagnostics.test.ts src/capture/messages.test.ts`

Run: `npm run typecheck --workspace apps/extension`

Expected: PASS; missing acknowledgements fail explicitly with a typed transport error.

- [x] **Step 8: Commit the content acknowledgement and diagnostics helper.**

```bash
git add apps/extension/src/capture/diagnostics.ts apps/extension/src/capture/diagnostics.test.ts apps/extension/src/capture/messages.ts apps/extension/src/capture/messages.test.ts apps/extension/entrypoints/content.ts
git commit -m "feat(extension): expose capture message acknowledgements"
```

### Task 2: Make background receipt and validation outcomes observable

**Files:**
- Create: `apps/extension/src/capture/background-messages.ts`
- Create: `apps/extension/src/capture/background-messages.test.ts`
- Modify: `apps/extension/entrypoints/background.ts`
- Reuse: `apps/extension/src/capture/diagnostics.ts`, `apps/extension/src/capture/messages.ts`

**Interfaces:**
- `CaptureMessageSender`: the subset `{ url?: string; tab?: { id?: number; url?: string } }` needed to validate a content-script sender.
- `handleCaptureEvent(message, sender, controller, diagnostics) -> Promise<CaptureEventReply | undefined>`: handles only `capture.event`; returns `undefined` for unrelated message types. For a recognized event, it always returns `{ accepted: true; gap: boolean }` or `{ error: string; code: string }`, including invalid sender/session and payload cases.
- `CaptureEventReply`: preserve the existing accepted/error envelope so the internal extension protocol remains compatible while making all outcomes explicit.

- [x] **Step 1: Add handler tests.** Cover valid click acceptance, missing tab ID, unusable sender URL, origin mismatch, missing/expired session, invalid event ID, and `controller.record` rejection. Verify the host recorder is called only for a valid sender and active, unexpired session.
- [x] **Step 2: Run the handler tests and confirm they fail.**

Run: `npm run test --workspace apps/extension -- src/capture/background-messages.test.ts`

Expected: FAIL because `handleCaptureEvent` is not implemented.

- [x] **Step 3: Extract the event branch from the background listener into `handleCaptureEvent`.** Keep validation and metadata allowlisting in the background boundary. A recognized `capture.event` must never fall through without a response; unrelated extension messages remain ignored.
- [x] **Step 4: Emit stage records for receipt, sender validation, session validation, record start, and record result.** Log stage/outcome and safe correlation only; use explicit error codes instead of raw page values.
- [x] **Step 5: Run handler tests and extension typecheck.**

Run: `npm run test --workspace apps/extension -- src/capture/background-messages.test.ts`

Run: `npm run typecheck --workspace apps/extension`

Expected: PASS; invalid recognized messages receive a safe error reply and valid messages receive an acknowledgement only after recording succeeds.

- [x] **Step 6: Commit the background receipt and validation diagnostics.**

```bash
git add apps/extension/src/capture/background-messages.ts apps/extension/src/capture/background-messages.test.ts apps/extension/entrypoints/background.ts
git commit -m "feat(extension): trace capture message validation"
```

### Task 3: Correlate native ingestion and expose host outcomes

**Files:**
- Modify: `apps/extension/src/capture/session.ts`
- Modify: `apps/extension/src/capture/session.test.ts`
- Modify: `apps/extension/src/native/client.ts`
- Create: `apps/extension/src/native/client.test.ts`
- Reuse: `apps/extension/src/capture/diagnostics.ts`

**Interfaces:**
- `CaptureController` accepts an optional diagnostics sink while preserving existing constructor behavior for tests and production callers.
- Around each click's `trace.ingest`, emit `host.ingest.started` and exactly one terminal stage: `host.ingest.accepted`, `host.ingest.rejected`, or `host.ingest.timeout`, correlated by `traceId`, `tabId`, and `eventId`.
- `NativeRequestError` carries one of `timeout`, `disconnected`, `post-failed`, or `host-rejected`. Native/core error text and payload content are not included in diagnostics.

- [x] **Step 1: Add `NativeClient` tests for timeout, disconnect, post failure, and a host error response.** Assert that each branch rejects with the corresponding `NativeRequestError` code.
- [x] **Step 2: Run the native-client tests and confirm they fail.**

Run: `npm run test --workspace apps/extension -- src/native/client.test.ts`

Expected: FAIL because `NativeRequestError` and typed failure mapping are not implemented.

- [x] **Step 3: Add `CaptureController` tests for accepted ingestion, native rejection, and a typed timeout.** Assert that diagnostics include the same event ID and trace ID, mark one terminal result, and never include `metadata.source` or the thrown error's raw message.
- [x] **Step 4: Run the focused session test and confirm the new cases fail.**

Run: `npm run test --workspace apps/extension -- src/capture/session.test.ts`

Expected: FAIL because host-stage diagnostics are not emitted.

- [x] **Step 5: Add `NativeRequestError` and map the existing `NativeClient` timeout, disconnect, post failure, and host error response to its four safe codes.** Preserve the existing error messages for internal debugging, but do not copy them into diagnostic records.
- [x] **Step 6: Add the optional diagnostics sink and bracket `trace.ingest` with stage records.** Use the result returned by the existing native command to distinguish accepted/duplicate counts when available; treat duplicate acceptance as a completed idempotent delivery, not a second event.
- [x] **Step 7: Preserve existing failure handling.** A native rejection or timeout must still mark the capture gap and return an error to the background; diagnostics must not swallow the failure or change sequence behavior.
- [x] **Step 8: Run native-client and session tests, the full extension test suite, and typecheck.**

Run: `npm run test --workspace apps/extension -- src/capture/session.test.ts`

Run: `npm run test --workspace apps/extension -- src/native/client.test.ts`

Run: `npm run test --workspace apps/extension`

Run: `npm run typecheck --workspace apps/extension`

Expected: PASS; success, duplicate, rejection, and timeout remain distinguishable.

- [x] **Step 9: Commit native-ingest outcome diagnostics.**

```bash
git add apps/extension/src/capture/session.ts apps/extension/src/capture/session.test.ts apps/extension/src/native/client.ts apps/extension/src/native/client.test.ts
git commit -m "feat(extension): report capture ingest outcomes"
```

### Task 4: Correlate the E2E assertion and decide whether transport redesign is warranted

**Files:**
- Modify: `tests/e2e/legacy-click.spec.ts`
- Modify: `tests/e2e/modern-click.spec.ts`
- Modify: `tests/e2e/support/capture.ts` only for a typed fixture-only diagnostic reader
- Modify: `apps/extension/entrypoints/background.ts` and `apps/extension/entrypoints/content.ts` only to expose bounded diagnostic records in fixture builds
- Reuse: `apps/extension/src/capture/diagnostics.ts`

**Interfaces:**
- Fixture-only `__legacyLensCaptureDiagnostics` ring with a fixed maximum of 100 background records; content-script stage records use a separate bounded `chrome.storage.local` ring. Both are unavailable in production builds.
- Test helper `captureDiagnostics(worker) -> Promise<CaptureStageRecord[]>` reads both rings directly through the service worker without sending another runtime message through the path being diagnosed.
- E2E success requires the same selected click ID to reach the host and appear in `investigation.get` for the expected trace. Stage records are attached to failure output without page or request contents.

- [x] **Step 1: Update the legacy E2E failure output to include fixture stage records and assert selection, explicit background acknowledgement, successful host ingest, and event visibility by the same event ID.**
- [x] **Step 2: Update the Jakarta E2E to assert `jsf.click` is present in its active trace and include stage records on failure.**
- [x] **Step 3: Run the existing Playwright test discovery command and fixture extension build locally.**

Run: `npm run test --workspace tests/e2e -- --list`

Run: `LEGACYLENS_FIXTURE_EXTENSION=1 npm run build --workspace apps/extension`

Expected: PASS; the tests are discovered, and fixture-only diagnostic access is included in the build.

- [x] **Step 4: Push the diagnostic implementation and inspect the Windows CI artifacts.**

Expected: Required checks pass. For each fixture, logs identify the last completed stage and either the exact safe error code or the matching persisted event. Do not report success based only on `legacylens:select` or Java agent events.

- [x] **Step 5: Apply the evidence gate.** Run `37818722260` showed `content.send.timeout` after 35,002 ms and no `background.receive` for the same `traceId`/`eventId`; therefore the failing boundary is the one-shot content→background path. Stop this diagnostic plan and review the separate Port follow-up plan before any transport implementation.

## Conditional follow-up: `chrome.runtime.Port`

Do not execute this section within the diagnostic implementation unless the Task 4 evidence gate attributes the failure to the one-shot transport and the user approves the follow-up plan. Before retries are added, preserve the event ID and verify core idempotency for the existing key `(projectId, traceId, producerId, eventId)`. The follow-up must define a bounded pending-event queue, ACK/NACK protocol, reconnect/stop/navigation lifecycle, and tests for duplicate delivery and worker restart.

## Self-review

- **Spec coverage:** Current message flow, silent no-reply paths, safe correlation, each boundary, conditional `Port` redesign, privacy limits, E2E success, and the evidence gate are covered by Tasks 1–4 or the conditional follow-up.
- **Step clarity:** Every test, implementation, verification, commit, and decision gate names its files, observable result, or command.
- **Type consistency:** The content and background share the accepted/error envelope; diagnostic storage carries only `CaptureStageRecord`; fixture tests read it directly from the service worker.
- **Review focus:** Missing sender, missing acknowledgement, inactive/expired session, host failures, and wrong trace/event correlation each map to a task test.
- **Proportion:** This plan implements only diagnostic instrumentation and the fix for the boundary identified by it. Port transport work stays conditional and receives a separate execution plan if the CI evidence requires it.
