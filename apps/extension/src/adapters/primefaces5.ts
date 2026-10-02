export interface AjaxAction {
  source?: string;
  url: string;
  headers: Record<string, string>;
  propagation: 'unlinked' | 'attempted' | 'propagated';
  traceparent?: string;
  spanId?: string;
}

export interface PageCaptureContext {
  origin: string;
  traceId: string;
  nextSpanId: () => string;
  onAjax: (action: AjaxAction) => void;
  onDiagnostic?: (code: string) => void;
  onNetwork?: (effect: import('../capture/network.ts').NetworkEffect) => void;
  withInteraction?: <T>(source: string, fn: () => T) => T;
}

type XHRLike = { setRequestHeader(name: string, value: string): unknown; send(...args: unknown[]): unknown };
type JQueryOptions = { url?: string; data?: unknown; headers?: Record<string, string>; xhr?: () => XHRLike };
type JQuery = { ajaxPrefilter(handler: (options: JQueryOptions) => void): void; ajaxSettings?: { xhr?: () => XHRLike } };
type Request = { handle(this: unknown, ...args: unknown[]): unknown };

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

function report(context: PageCaptureContext, action: AjaxAction): void {
  try { context.onAjax(action); } catch { diagnostic(context, 'PRIMEFACES_OBSERVER_ERROR'); }
}

export class PrimeFacesAdapter {
  private context: PageCaptureContext | undefined;
  private selected: string | undefined;
  private invoking: string | undefined;

  install(context: PageCaptureContext): () => void {
    this.context = context;
    const page = globalThis as typeof globalThis & { PrimeFaces?: { VERSION?: unknown; ajax?: { Request?: Request } }; jQuery?: JQuery };
    const request = page.PrimeFaces?.ajax?.Request;
    if (page.PrimeFaces?.VERSION == null) diagnostic(context, 'PRIMEFACES_VERSION_EXPERIMENTAL');
    if (typeof request?.handle !== 'function' || typeof page.jQuery?.ajaxPrefilter !== 'function') {
      diagnostic(context, 'UNSUPPORTED_PRIMEFACES_PAGE');
      this.context = undefined;
      return () => {};
    }
    const originalHandle = request.handle;
    const adapter = this;
    const wrappedHandle: Request['handle'] = function (...args) {
      const cfg = args[0] as { source?: unknown; s?: unknown } | undefined;
      const source = typeof cfg?.source === 'string' ? cfg.source : typeof cfg?.s === 'string' ? cfg.s : undefined;
      const previous = adapter.invoking;
      if (source && source === adapter.selected) adapter.invoking = source;
      try {
        const invoke = () => Reflect.apply(originalHandle, this, args);
        return source && source === adapter.selected && context.withInteraction
          ? context.withInteraction(source, invoke) : invoke();
      }
      finally { adapter.invoking = previous; }
    };
    request.handle = wrappedHandle;
    let enabled = true;
    page.jQuery.ajaxPrefilter((options) => {
      if (!enabled || !this.context || !this.invoking) return;
      try {
        const source = sourceFrom(options.data);
        if (!source || source !== this.invoking) return;
        const action = this.observeAjax({ source, url: options.url ?? location.href, headers: options.headers ?? {}, propagation: 'unlinked' });
        if (action.propagation !== 'attempted' || !action.traceparent || !action.spanId) return;
        options.headers = action.headers;
        const traceparent = action.traceparent;
        const spanId = action.spanId;
        report(context, { source, url: action.url, headers: { ...action.headers }, propagation: 'attempted' });
        const factory = options.xhr ?? page.jQuery?.ajaxSettings?.xhr;
        if (typeof factory !== 'function') return;
        options.xhr = function () {
          const xhr = Reflect.apply(factory, this, []) as XHRLike;
          const originalSet = xhr.setRequestHeader;
          const originalSend = xhr.send;
          if (typeof originalSet !== 'function' || typeof originalSend !== 'function') return xhr;
          const headers = new Map<string, string>();
          try {
            xhr.setRequestHeader = function (name, value) {
              const result = Reflect.apply(originalSet, this, [name, value]);
              const key = name.toLowerCase();
              const previous = headers.get(key);
              headers.set(key, previous === undefined ? value : `${previous}, ${value}`);
              return result;
            };
            xhr.send = function (...args) {
              const result = Reflect.apply(originalSend, this, args);
              if (headers.get('traceparent') === traceparent) {
                report(context, { source, url: action.url, headers: { ...action.headers }, propagation: 'propagated', traceparent, spanId });
              }
              return result;
            };
          } catch { diagnostic(context, 'PRIMEFACES_TRANSPORT_UNOBSERVED'); }
          return xhr;
        };
      } catch { diagnostic(context, 'PRIMEFACES_HOOK_ERROR'); }
    });
    return () => {
      enabled = false;
      if (request.handle === wrappedHandle) request.handle = originalHandle;
      this.context = undefined;
      this.selected = undefined;
      this.invoking = undefined;
    };
  }

  select(jsfClientId: string): void {
    if (!jsfClientId) return;
    this.selected = jsfClientId;
    setTimeout(() => { if (this.selected === jsfClientId) this.selected = undefined; }, 0);
  }

  observeAjax(action: AjaxAction): AjaxAction {
    const context = this.context;
    if (!context) return action;
    const result = { ...action, headers: { ...action.headers } };
    let url: URL;
    try { url = new URL(action.url, context.origin); } catch { diagnostic(context, 'PRIMEFACES_URL_INVALID'); return result; }
    if (action.source === this.selected && action.source === this.invoking && url.origin === context.origin
      && !Object.keys(result.headers).some((key) => key.toLowerCase() === 'traceparent')) {
      const spanId = context.nextSpanId();
      if (/^[a-f0-9]{32}$/i.test(context.traceId) && /^[a-f0-9]{16}$/i.test(spanId) && !/^0+$/.test(spanId)) {
        result.propagation = 'attempted';
        result.traceparent = `00-${context.traceId}-${spanId}-01`;
        result.spanId = spanId;
        result.headers.traceparent = result.traceparent;
        this.selected = undefined;
      }
    }
    if (result.propagation === 'unlinked') report(context, result);
    return result;
  }
}
