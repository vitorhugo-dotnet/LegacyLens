import { defineContentScript } from 'wxt/utils/define-content-script';
import { PrimeFacesAdapter } from '../src/adapters/primefaces5.ts';

export default defineContentScript({
  matches: ['http://*/*', 'https://*/*'],
  world: 'MAIN',
  runAt: 'document_start',
  main() {
    const adapter = new PrimeFacesAdapter();
    let uninstall: (() => void) | undefined;
    let nonce = '';
    window.addEventListener('legacylens:start', (event) => {
      const detail = (event as CustomEvent).detail as { nonce?: string; traceId?: string; origin?: string };
      if (!detail || typeof detail.nonce !== 'string' || !/^[a-f0-9]{32}$/i.test(detail.traceId ?? '') || detail.origin !== location.origin) return;
      uninstall?.(); nonce = detail.nonce;
      uninstall = adapter.install({ origin: location.origin, traceId: detail.traceId!,
        nextSpanId: () => [...crypto.getRandomValues(new Uint8Array(8))].map((n) => n.toString(16).padStart(2, '0')).join(''),
        onAjax: (action) => window.dispatchEvent(new CustomEvent('legacylens:ajax', { detail: { nonce, source: action.source, traceparent: action.traceparent, spanId: action.spanId } })),
        onDiagnostic: (code) => window.dispatchEvent(new CustomEvent('legacylens:ajax', { detail: { nonce, code } })),
      });
    });
    window.addEventListener('legacylens:select', (event) => {
      const detail = (event as CustomEvent).detail as { nonce?: string; source?: string };
      if (detail?.nonce === nonce && typeof detail.source === 'string') adapter.select(detail.source);
    });
    window.addEventListener('legacylens:stop', (event) => {
      if ((event as CustomEvent).detail?.nonce === nonce) { uninstall?.(); uninstall = undefined; nonce = ''; }
    });
  },
});
