import { defineContentScript } from 'wxt/utils/define-content-script';
import { selectPrimeFacesAdapter, type PagePrimeFacesAdapter } from '../src/adapters/registry.ts';
import { armInteraction, installAsyncContextCapture, withInteraction, withoutInteraction } from '../src/capture/async-context.ts';
import { installNetworkCapture } from '../src/capture/network.ts';

export default defineContentScript({
  matches: ['http://*/*', 'https://*/*'],
  world: 'MAIN',
  runAt: 'document_start',
  main() {
    const global = globalThis as typeof globalThis & { __legacylensPageContentInstalled?: boolean };
    if (global.__legacylensPageContentInstalled) return;
    global.__legacylensPageContentInstalled = true;

    // Install before page scripts register listeners; selection arms one matching click later.
    installAsyncContextCapture();
    let adapter: PagePrimeFacesAdapter | undefined;
    let uninstall: (() => void) | undefined;
    let nonce = '';
    let traceId = '';
    window.addEventListener('legacylens:start', (event) => {
      const detail = (event as CustomEvent).detail as { nonce?: string; traceId?: string; origin?: string };
      if (!detail || typeof detail.nonce !== 'string' || !/^[a-f0-9]{32}$/i.test(detail.traceId ?? '') || detail.origin !== location.origin) return;
      uninstall?.(); nonce = detail.nonce; traceId = detail.traceId!;
      const page = globalThis as typeof globalThis & {
        PrimeFaces?: { VERSION?: unknown; ajax?: { Request?: { handle?: unknown } } };
        jQuery?: { ajaxPrefilter?: unknown };
      };
      adapter = selectPrimeFacesAdapter(page);
      const nextSpanId = () => {
        let id: string;
        do { id = [...crypto.getRandomValues(new Uint8Array(8))].map((n) => n.toString(16).padStart(2, '0')).join(''); }
        while (/^0+$/.test(id));
        return id;
      };
      const context = { origin: location.origin, traceId,
        nextSpanId,
        onAjax: (action: import('../src/adapters/primefaces5.ts').AjaxAction) => withoutInteraction(() => window.dispatchEvent(new CustomEvent('legacylens:ajax', { detail: { nonce, source: action.source, propagation: action.propagation, traceparent: action.traceparent, spanId: action.spanId } }))),
        onNetwork: (effect: import('../src/capture/network.ts').NetworkEffect) => withoutInteraction(() => window.dispatchEvent(new CustomEvent('legacylens:network', { detail: { nonce, ...effect } }))),
        onDiagnostic: (code: string) => withoutInteraction(() => window.dispatchEvent(new CustomEvent('legacylens:ajax', { detail: { nonce, code } }))),
        withInteraction: <T>(source: string, fn: () => T): T => withInteraction({ source, origin: location.origin, traceId }, fn),
      };
      const stopNetwork = installNetworkCapture(context);
      const stopPrimeFaces = adapter.install(context);
      uninstall = () => { stopPrimeFaces(); stopNetwork(); };
    });
    window.addEventListener('legacylens:select', (event) => {
      const detail = (event as CustomEvent).detail as { nonce?: string; source?: string };
      if (detail?.nonce === nonce && typeof detail.source === 'string') {
        adapter?.select(detail.source);
        armInteraction({ source: detail.source, origin: location.origin, traceId });
      }
    });
    window.addEventListener('legacylens:stop', (event) => {
      if ((event as CustomEvent).detail?.nonce === nonce) { uninstall?.(); uninstall = undefined; adapter = undefined; nonce = ''; traceId = ''; }
    });
  },
});
