import type { AjaxAction, PageCaptureContext } from './primefaces5.ts';

type XHRLike = { setRequestHeader(name: string, value: string): unknown; send(...args: unknown[]): unknown };
type JQueryOptions = { url?: string; data?: unknown; headers?: Record<string, string>; xhr?: () => XHRLike };
type JQuery = { ajaxPrefilter(handler: (options: JQueryOptions) => void): void; ajaxSettings?: { xhr?: () => XHRLike } };
type RemoteCommand = { ab(this: unknown, ...args: unknown[]): unknown };
type PrimeFaces15Runtime = { VERSION?: unknown; ab?: RemoteCommand['ab'] };

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

function isPrimeFaces15(version: unknown): boolean {
  return typeof version === 'string' && /^15(?:\.|$)/.test(version);
}

function isKnownPrimeFaces5(version: unknown): boolean {
  return typeof version === 'string' && /^5(?:\.|$)/.test(version);
}

export class PrimeFaces15Adapter {
  private context: PageCaptureContext | undefined;
  private selected: string | undefined;
  private invoking: string | undefined;

  install(context: PageCaptureContext): () => void {
    this.context = context;
    const page = globalThis as typeof globalThis & { PrimeFaces?: PrimeFaces15Runtime; jQuery?: JQuery };
    const primeFaces = page.PrimeFaces;
    if (isKnownPrimeFaces5(primeFaces?.VERSION)) {
      diagnostic(context, 'UNSUPPORTED_PRIMEFACES_VERSION');
      this.context = undefined;
      return () => {};
    }
    if (!isPrimeFaces15(primeFaces?.VERSION)) diagnostic(context, 'PRIMEFACES_VERSION_EXPERIMENTAL');

    const ajax = page.jQuery;
    const originalAb = primeFaces?.ab;
    if (!primeFaces || typeof originalAb !== 'function' || typeof ajax?.ajaxPrefilter !== 'function') {
      diagnostic(context, 'UNSUPPORTED_PRIMEFACES_PAGE');
      this.context = undefined;
      return () => {};
    }

    const adapter = this;
    const wrappedAb: RemoteCommand['ab'] = function (...args) {
      const cfg = args[0] as { s?: unknown; source?: unknown } | undefined;
      const source = typeof cfg?.s === 'string' ? cfg.s : typeof cfg?.source === 'string' ? cfg.source : undefined;
      const selected = !!source && source === adapter.selected;
      const previous = adapter.invoking;
      if (selected) adapter.invoking = source;
      try {
        const invoke = () => Reflect.apply(originalAb, this, args);
        return selected && context.withInteraction
          ? context.withInteraction(source!, invoke) : invoke();
      } finally { adapter.invoking = previous; }
    };
    primeFaces.ab = wrappedAb;

    let enabled = true;
    ajax.ajaxPrefilter((options) => {
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
        const factory = options.xhr ?? ajax.ajaxSettings?.xhr;
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
      if (primeFaces.ab === wrappedAb) primeFaces.ab = originalAb;
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
