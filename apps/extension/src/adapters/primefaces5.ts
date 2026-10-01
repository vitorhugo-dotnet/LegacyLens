export interface AjaxAction {
  source?: string;
  url: string;
  headers: Record<string, string>;
  traceparent?: string;
  spanId?: string;
}

export interface PageCaptureContext {
  origin: string;
  traceId: string;
  nextSpanId: () => string;
  onAjax: (action: AjaxAction) => void;
  onDiagnostic?: (code: string) => void;
}

type JQueryOptions = { url?: string; data?: unknown; headers?: Record<string, string> };
type JQuery = { ajaxPrefilter: (handler: (options: JQueryOptions) => void) => void };

function sourceFrom(data: unknown): string | undefined {
  if (typeof data === 'string') return new URLSearchParams(data).get('javax.faces.source') ?? undefined;
  if (data && typeof data === 'object' && 'javax.faces.source' in data) {
    const source = (data as Record<string, unknown>)['javax.faces.source'];
    return typeof source === 'string' ? source : undefined;
  }
  return undefined;
}

function diagnostic(context: PageCaptureContext, code: string): void {
  try { context.onDiagnostic?.(code); } catch { /* Observation must not interrupt the application. */ }
}

export class PrimeFacesAdapter {
  private context: PageCaptureContext | undefined;
  private selected: string | undefined;

  install(context: PageCaptureContext): () => void {
    this.context = context;
    const page = globalThis as typeof globalThis & { PrimeFaces?: { ajax?: { Request?: { handle?: unknown } } }; jQuery?: JQuery };
    if (typeof page.PrimeFaces?.ajax?.Request?.handle !== 'function' || typeof page.jQuery?.ajaxPrefilter !== 'function') {
      diagnostic(context, 'UNSUPPORTED_PRIMEFACES_PAGE');
      this.context = undefined;
      return () => {};
    }
    page.jQuery.ajaxPrefilter((options) => {
      if (!this.context) return;
      try {
        const source = sourceFrom(options.data);
        if (!source) return;
        const action = this.observeAjax({ source, url: options.url ?? location.href, headers: options.headers ?? {} });
        if (action.traceparent) options.headers = action.headers;
      } catch { diagnostic(context, 'PRIMEFACES_HOOK_ERROR'); }
    });
    return () => { this.context = undefined; this.selected = undefined; };
  }

  select(jsfClientId: string): void {
    if (!jsfClientId) return;
    this.selected = jsfClientId;
    queueMicrotask(() => { if (this.selected === jsfClientId) this.selected = undefined; });
  }

  observeAjax(action: AjaxAction): AjaxAction {
    const context = this.context;
    if (!context) return action;
    const result = { ...action, headers: { ...action.headers } };
    let url: URL;
    try { url = new URL(action.url, context.origin); } catch { diagnostic(context, 'PRIMEFACES_URL_INVALID'); return result; }
    if (action.source === this.selected && url.origin === context.origin && !Object.keys(result.headers).some((key) => key.toLowerCase() === 'traceparent')) {
      const spanId = context.nextSpanId();
      if (/^[a-f0-9]{32}$/i.test(context.traceId) && /^[a-f0-9]{16}$/i.test(spanId) && !/^0+$/.test(spanId)) {
        result.traceparent = `00-${context.traceId}-${spanId}-01`;
        result.spanId = spanId;
        result.headers.traceparent = result.traceparent;
        this.selected = undefined;
      }
    }
    try { context.onAjax(result); } catch { diagnostic(context, 'PRIMEFACES_OBSERVER_ERROR'); }
    return result;
  }
}
