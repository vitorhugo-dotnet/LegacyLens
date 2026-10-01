import { defineContentScript } from 'wxt/utils/define-content-script';
import type { ProjectListResult, CaptureSession } from '@legacylens/contracts/src/protocol.ts';
import { chooseElement } from '../src/capture/selection.ts';

type Reply<T> = T | { error: string };
async function ask<T>(message: object): Promise<T> {
  const reply = await chrome.runtime.sendMessage(message) as Reply<T>;
  if (reply && typeof reply === 'object' && 'error' in reply) throw new Error(reply.error);
  return reply as T;
}

export default defineContentScript({
  matches: ['http://*/*', 'https://*/*'],
  runAt: 'document_start',
  main() {
    let session: CaptureSession | undefined;
    let nonce = '';
    let selectedSource = '';
    const open = async () => {
      const existing = document.querySelector('[data-legacylens-ui]');
      if (existing) { existing.remove(); return; }
      const panel = document.createElement('div');
      panel.dataset.legacylensUi = 'true';
      Object.assign(panel.style, { position: 'fixed', top: '12px', right: '12px', zIndex: '2147483647', background: '#fff', color: '#111', border: '2px solid #3677db', padding: '12px', font: '14px sans-serif' });
      const label = document.createElement('label'); label.textContent = 'LegacyLens project: ';
      const select = document.createElement('select');
      const start = document.createElement('button'); start.textContent = 'Capture next interaction';
      const stop = document.createElement('button'); stop.textContent = 'Stop capture';
      const status = document.createElement('div'); status.setAttribute('role', 'status');
      label.append(select); panel.append(label, start, stop, status); document.documentElement.append(panel);
      try {
        const projects = await ask<ProjectListResult>({ type: 'projects.list' });
        for (const project of projects.items) { const option = document.createElement('option'); option.value = project.id; option.textContent = project.name; select.append(option); }
        if (projects.hasMore) status.textContent = 'Showing first 200 projects.';
        if (!projects.items.length) status.textContent = 'Register a project with the local LegacyLens CLI first.';
      } catch { status.textContent = 'Local LegacyLens host unavailable.'; }
      start.onclick = async () => {
        if (!select.value) return;
        try {
          const reply = await ask<{ session: CaptureSession }>({ type: 'capture.start', projectId: select.value });
          session = reply.session;
          nonce = [...crypto.getRandomValues(new Uint8Array(16))].map((n) => n.toString(16).padStart(2, '0')).join('');
          window.dispatchEvent(new CustomEvent('legacylens:start', { detail: { nonce, traceId: session.id, origin: location.origin } }));
          status.textContent = 'Click the JSF element to capture its next Ajax action.';
          chooseElement(document, (id) => {
            selectedSource = id;
            queueMicrotask(() => { if (selectedSource === id) selectedSource = ''; });
            window.dispatchEvent(new CustomEvent('legacylens:select', { detail: { nonce, source: id } }));
            void ask({ type: 'capture.event', sessionId: session!.id, kind: 'jsf.click', metadata: { source: id } }).catch(() => { status.textContent = 'Capture transport interrupted; investigation has a gap.'; });
            status.textContent = `Selected ${id}. Capture continues until stopped.`;
          });
        } catch (error) { status.textContent = error instanceof Error ? error.message : 'Capture failed'; }
      };
      stop.onclick = async () => {
        try { await ask({ type: 'capture.stop' }); session = undefined; window.dispatchEvent(new CustomEvent('legacylens:stop', { detail: { nonce } })); status.textContent = 'Capture stopped.'; }
        catch { status.textContent = 'Could not stop capture; retry.'; }
      };
    };
    chrome.runtime.onMessage.addListener((message: unknown) => { if ((message as { type?: string })?.type === 'selection.open') void open(); });
    window.addEventListener('legacylens:ajax', (event) => {
      const detail = (event as CustomEvent).detail as { nonce?: string; source?: string; traceparent?: string; spanId?: string; code?: string };
      if (!session || !detail || detail.nonce !== nonce) return;
      if (typeof detail.code === 'string' && /^UNSUPPORTED_[A-Z_]+$/.test(detail.code)) {
        void ask({ type: 'capture.event', sessionId: session.id, kind: 'extension.diagnostic', metadata: { code: detail.code } }).catch(() => {});
        const status = document.querySelector('[data-legacylens-ui] [role="status"]');
        if (status) status.textContent = 'This page does not expose the supported PrimeFaces 5 Ajax API.';
        return;
      }
      if (typeof detail.source !== 'string' || detail.source.length > 256) return;
      if (detail.source !== selectedSource || !detail.traceparent || !detail.spanId) return;
      if (detail.traceparent !== `00-${session.id}-${detail.spanId}-01` || !/^[a-f0-9]{16}$/i.test(detail.spanId)) return;
      void ask({ type: 'capture.event', sessionId: session.id, kind: 'primefaces.ajax', metadata: { source: detail.source, ...(detail.spanId && /^[a-f0-9]{16}$/i.test(detail.spanId) ? { spanId: detail.spanId } : {}) } }).catch(() => {
        const status = document.querySelector('[data-legacylens-ui] [role="status"]');
        if (status) status.textContent = 'Capture transport interrupted; investigation has a gap.';
      });
    });
  },
});
