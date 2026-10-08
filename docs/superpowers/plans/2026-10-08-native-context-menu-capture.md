# Native Context Menu Capture Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Replace the injected capture panel with a native LegacyLens context-menu submenu that selects a project per tab, starts/stops capture, and opens project management; explain installation and host diagnosis to users.

**Architecture:** The background worker owns the native submenu, selected-project/session state, host requests, permissions, and navigation. The content scripts remain event instrumentation only: `content.ts` receives typed start/stop commands and forwards selected interaction and Ajax/network events; `page.content.ts` instruments the page without drawing controls. `investigation.html` remains the project registration/index/search surface.

**Tech Stack:** WXT, TypeScript, Chrome MV3 `contextMenus`/`storage.session`/`scripting`/`tabs` APIs, existing `NativeClient` and `CaptureController`, Markdown documentation.

**Spec path:** `docs/superpowers/specs/2026-10-08-native-context-menu-capture-design.md`

**Global Constraints:** Do not add a page-injected popup or controls. The Start command must request the optional site permission in the menu click gesture before any `await`. Keep project selection scoped to a tab and active captures scoped to their existing `CaptureController` state. Make script injection and command handling idempotent. Distinguish native-host connection failure from an empty project list. No automated tests are added or run in this implementation; verify with the extension build and the manual acceptance flow below.

**Review Focus:** Chrome's context-menu lifecycle and `refresh()` behavior; permission request still occurs in the direct click gesture; menu item IDs safely represent arbitrary project IDs; start/stop state stays consistent across tab navigation and service-worker restarts; content scripts leave no visual UI and clean up listeners/state; Windows instructions correctly explain the host registration and PowerShell policy failure.

## Task 1: Define capture commands and remove the injected panel

**Files:** Modify `apps/extension/entrypoints/content.ts`; modify `apps/extension/entrypoints/page.content.ts`; create `apps/extension/src/extension-messages.ts` if shared message types are needed.

**Interfaces:** Consume background commands `capture.begin` (`projectId`, `CaptureSession`) and `capture.end` (`traceId`). Produce `capture.event` messages already consumed by `background.ts`, preserving existing event identity and metadata validation. The page-world handshake remains `legacylens:start`, `legacylens:select`, and `legacylens:stop`.

- [x] **Step 1: Extract capture lifecycle from the panel.** Move the current `start.onclick` setup into an idempotent `beginCapture(projectId, session)` routine. It initializes session/nonce/selection, dispatches the existing page-world start event, and calls `chooseElement`; it must not create DOM nodes.
- [x] **Step 2: Add background command handling.** Listen for `capture.begin` and `capture.end`, validate the session shape and current origin, and start/clear content-side capture state. Keep diagnostics and transport failures in extension-side logging or messages; do not render a popup/status element into the application DOM.
- [x] **Step 3: Remove panel-only code.** Delete `open()`, select/buttons/status construction, and `selection.open` handling. Keep the installed marker and page instrumentation listeners safe against duplicate injection.
- [x] **Step 4: Make page instrumentation cleanup complete.** Ensure stop removes adapter/network hooks and resets adapter, nonce, and trace state; repeated start and stop messages do not multiply listeners.
- [x] **Step 5: Build the extension.** Run `npm run build --workspace apps/extension`; confirm TypeScript/WXT compilation succeeds and no popup code remains in the emitted capture entrypoint.
- [x] **Step 6: Commit.** Use `git add` on only the listed source files and commit as `feat(extension): remove injected capture panel`.

## Task 2: Implement the project-aware native submenu and capture lifecycle

**Files:** Modify `apps/extension/entrypoints/background.ts`; modify `apps/extension/wxt.config.ts`; modify `apps/extension/src/extension-messages.ts` if created in Task 1.

**Interfaces:** Query `NativeClient.request<ProjectListResult>('project.list', { offset: 0, limit: 200 })`. Use `CaptureController.start({ projectId, tabId, origin })`, `get(tabId)`, and `stop(tabId)`. Send `capture.begin`/`capture.end` commands to the tab content script. Store the selected project map under a versioned `chrome.storage.session` key keyed by tab ID.

- [x] **Step 1: Create stable parent and action menu items.** On install/startup create one `LegacyLens` parent with stable child IDs for Start, Stop, and Manage. Restrict it to `page` contexts on HTTP/HTTPS. Recreate idempotently so worker restarts do not duplicate entries.
- [x] **Step 2: Refresh dynamic project radio items and action state on menu display.** On `contextMenus.onShown`, query the host, replace only tracked dynamic project items, add radio items with IDs derived from validated project IDs, check the selected project's item for that tab, and enable Start/Stop/Manage according to host, selection, and active-capture state; then call `contextMenus.refresh()`. Retain project names as titles, not IDs.
- [x] **Step 3: Represent host and empty-project states separately.** If `project.list` fails, show a disabled `Host nativo desconectado` entry. If the host responds with no projects, show a disabled no-project entry plus Manage. Never convert a failed request into an empty list.
- [x] **Step 4: Persist per-tab project selection.** When a radio item is clicked, update the tab-ID mapping in `chrome.storage.session`, then refresh the menu's checked state. Clear a tab's selection when the tab is removed; validate stored IDs against the latest host result before enabling Start.
- [x] **Step 5: Start capture in the direct menu gesture.** In the Start click handler, call `chrome.permissions.request` for the active page origin before awaiting other work. Ensure a project is selected and no capture is active; inject `content-scripts/content.js` and `content-scripts/page.js` idempotently, call `CaptureController.start`, and send `capture.begin`. If a later step fails, stop the just-created session and surface a badge/error without inserting UI.
- [x] **Step 6: Stop and navigate to investigation.** On Stop, call `CaptureController.stop`, send `capture.end` for page cleanup, then open `investigation.html` with `projectId` and `traceId`. If a service-worker restart restored an active capture but content script is absent, stop still closes the native session and opens the investigation.
- [x] **Step 7: Wire Manage and toolbar icon.** Manage opens `investigation.html`; toolbar click opens the same page and never injects scripts or requests page permissions. Remove the old `selection.open` menu action.
- [x] **Step 8: Build and inspect the generated manifest.** Run `npm run build --workspace apps/extension`; confirm MV3 manifest includes `contextMenus`, `storage`, `scripting`, `tabs`, and the existing optional HTTP/HTTPS host permissions. Inspect the generated background bundle for the fixed parent/actions and ensure no `selection.open` path remains.
- [x] **Step 9: Commit.** Stage only background/config/message source files and commit as `feat(extension): add project capture submenu`.

## Task 3: Document user workflow and native-host recovery

**Files:** Modify `README.md`; modify `docs/user/windows-installation.md`.

**Interfaces:** Documentation must match the submenu labels and the actual `legacylens.exe` CLI and PowerShell installer parameters.

- [x] **Step 1: Rewrite README “Como usar”.** Explain extracting the release ZIP, loading `extension` in Chrome/Edge, registering the host using the current extension ID, registering a project or using Manage projects, starting `legacylens.exe serve`, choosing a project per tab, starting/stopping from right-click > LegacyLens, and using the investigation page. Explain that toolbar click opens management.
- [x] **Step 2: Add Windows host troubleshooting.** Document the difference between `legacylens.exe status` (HTTP core health) and extension native-host connectivity, expected disabled menu status, and how to confirm the extension ID and registry registration. For PowerShell execution-policy blocks, show `Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass` followed by the existing `install-native-host.ps1` invocation; state that this affects only the current PowerShell process.
- [x] **Step 3: Check links and command consistency.** Confirm README links to `docs/user/windows-installation.md`, package paths and script parameters match the repository, and the instructions make no claim that a healthy HTTP core proves Native Messaging is installed.
- [x] **Step 4: Commit.** Stage only the documentation files and commit as `docs: explain context menu capture and host setup`.

## Task 4: End-to-end manual acceptance

**Files:** No additional source files unless acceptance exposes an implementation defect; any correction belongs in its owning task and commit.

**Interfaces:** Use the built unpacked Chrome/Edge extension, a registered native host, running LegacyLens core, and one registered local project.

- [ ] **Step 1: Verify disconnected host state.** With the HTTP core healthy but the host not registered/available, open a page context menu and confirm it reports `Host nativo desconectado`, rather than presenting an empty project list.
- [ ] **Step 2: Verify project and capture flow.** Register the host and project; confirm the project radio item appears and selection on one tab does not change another tab. Start capture, authorize site access when prompted, select an element, perform an interaction, stop capture, and confirm the investigation opens for the captured project/trace.
- [ ] **Step 3: Verify management and no injected UI.** Confirm Manage and toolbar click open `investigation.html`; the application page never displays a LegacyLens panel or controls. Reopen the context menu repeatedly and confirm there is only one parent and one set of actions.
- [x] **Step 4: Record manual outcome.** Not run: this environment has no access to the user’s Windows Chrome/Edge profile or registered native host, so browser acceptance still needs confirmation there.

## Self-review

- Spec coverage: native submenu, radio selection per tab, start/stop, management, toolbar behavior, no page UI, host error state, host setup documentation, and build/manifest acceptance are all represented.
- Step ordering: content lifecycle protocol is defined before the background sends commands; menu state and capture integration precede documentation and manual acceptance.
- Interface consistency: menu starts use the existing `CaptureController`; content events preserve the existing validated `capture.event` contract; stop navigation uses the existing investigation query parameters.
- Highest-risk items have explicit checks: user-gesture permission request (Task 2 Step 5); host error vs empty list (Task 2 Step 3 / Task 4 Step 1); per-tab selection (Task 2 Step 4 / Task 4 Step 2); idempotent content cleanup (Task 1 Steps 3–4 / Task 4 Step 3); PowerShell host recovery (Task 3 Step 2).
- Verification scope follows the instruction not to add or run automated tests. Typecheck/build and manifest inspection passed; manual browser acceptance remains unverified because the target Windows browser and native host are unavailable here.
