import type { PageCaptureContext } from '../adapters/primefaces5.ts';
import { currentInteraction, withoutInteraction, type InteractionContext } from './async-context.ts';
import { parseStack, type SourceFrame } from './stack.ts';

export interface NetworkEffect {
  transport: 'fetch' | 'xhr';
  source: string;
  propagation: 'attempted' | 'propagated';
  spanId?: string;
  traceparent?: string;
  frames: SourceFrame[];
}

function linked(context: PageCaptureContext, interaction?: InteractionContext): interaction is InteractionContext {
  return !!interaction && interaction.origin === context.origin && interaction.traceId === context.traceId;
}

function sameOrigin(url: string, origin: string): boolean {
  try { return new URL(url, origin).origin === origin; } catch { return false; }
}

function traceparent(context: PageCaptureContext): { spanId: string; value: string } | undefined {
  const spanId = context.nextSpanId();
  if (!/^[a-f0-9]{32}$/i.test(context.traceId) || /^0+$/.test(context.traceId)
    || !/^[a-f0-9]{16}$/i.test(spanId) || /^0+$/.test(spanId)) return;
  return { spanId, value: `00-${context.traceId}-${spanId}-01` };
}

function alreadyLinked(header: string, traceId: string): boolean {
  return new RegExp(`^00-${traceId}-[a-f0-9]{16}-01$`, 'i').test(header);
}

function report(context: PageCaptureContext, effect: NetworkEffect): void {
  try { withoutInteraction(() => context.onNetwork?.(effect)); } catch { /* Observer cannot change application behavior. */ }
}

export function installNetworkCapture(context: PageCaptureContext): () => void {
  const originalFetch = globalThis.fetch;
  const xhrPrototype = globalThis.XMLHttpRequest?.prototype;
  const originalOpen = xhrPrototype?.open;
  const originalSet = xhrPrototype?.setRequestHeader;
  const originalSend = xhrPrototype?.send;
  const xhrState = new WeakMap<XMLHttpRequest, { url: string; interaction: InteractionContext | undefined; headers: Map<string, string> }>();
  let enabled = true;

  const wrappedFetch: typeof fetch = function (this: typeof globalThis, input, init) {
    const interaction = currentInteraction();
    if (!enabled || !linked(context, interaction)) return Reflect.apply(originalFetch, this, [input, init]);
    const url = input instanceof Request ? input.url : String(input);
    if (!sameOrigin(url, context.origin)) return Reflect.apply(originalFetch, this, [input, init]);
    const frames = parseStack(new Error().stack ?? '');
    const headers = new Headers(init?.headers ?? (input instanceof Request ? input.headers : undefined));
    if (headers.has('traceparent')) {
      if (!alreadyLinked(headers.get('traceparent') ?? '', context.traceId))
        report(context, { transport: 'fetch', source: interaction.source, propagation: 'attempted', frames });
      return Reflect.apply(originalFetch, this, [input, init]);
    }
    let identity: ReturnType<typeof traceparent>;
    try { identity = traceparent(context); } catch { return Reflect.apply(originalFetch, this, [input, init]); }
    if (!identity) return Reflect.apply(originalFetch, this, [input, init]);
    headers.set('traceparent', identity.value);
    const result = Reflect.apply(originalFetch, this, [input, { ...init, headers }]);
    report(context, { transport: 'fetch', source: interaction.source, propagation: 'propagated', spanId: identity.spanId,
      traceparent: identity.value, frames });
    return result;
  };
  globalThis.fetch = wrappedFetch;

  const wrappedOpen: typeof XMLHttpRequest.prototype.open = function (this: XMLHttpRequest, method: string, url: string | URL, ...args: any[]) {
    const result = Reflect.apply(originalOpen, this, [method, url, ...args]);
    if (enabled) xhrState.set(this, { url: String(url), interaction: currentInteraction(), headers: new Map() });
    return result;
  };
  const wrappedSet: typeof XMLHttpRequest.prototype.setRequestHeader = function (this: XMLHttpRequest, name, value) {
    const result = Reflect.apply(originalSet, this, [name, value]);
    const headers = xhrState.get(this)?.headers;
    if (enabled && headers) {
      const key = name.toLowerCase();
      const previous = headers.get(key);
      headers.set(key, previous === undefined ? value : `${previous}, ${value}`);
    }
    return result;
  };
  const wrappedSend: typeof XMLHttpRequest.prototype.send = function (this: XMLHttpRequest, ...args) {
    const state = xhrState.get(this);
    const interaction = currentInteraction() ?? state?.interaction;
    if (!enabled || !state || !linked(context, interaction) || !sameOrigin(state.url, context.origin))
      return Reflect.apply(originalSend, this, args);
    const frames = parseStack(new Error().stack ?? '');
    if (state.headers.has('traceparent')) {
      if (!alreadyLinked(state.headers.get('traceparent') ?? '', context.traceId))
        report(context, { transport: 'xhr', source: interaction.source, propagation: 'attempted', frames });
      return Reflect.apply(originalSend, this, args);
    }
    let identity: ReturnType<typeof traceparent>;
    try { identity = traceparent(context); } catch { return Reflect.apply(originalSend, this, args); }
    if (!identity) return Reflect.apply(originalSend, this, args);
    try { Reflect.apply(originalSet, this, ['traceparent', identity.value]); }
    catch { report(context, { transport: 'xhr', source: interaction.source, propagation: 'attempted', frames });
      return Reflect.apply(originalSend, this, args); }
    state.headers.set('traceparent', identity.value);
    const result = Reflect.apply(originalSend, this, args);
    if (state.headers.get('traceparent') === identity.value)
      report(context, { transport: 'xhr', source: interaction.source, propagation: 'propagated', spanId: identity.spanId,
        traceparent: identity.value, frames });
    return result;
  };
  if (xhrPrototype) {
    xhrPrototype.open = wrappedOpen;
    xhrPrototype.setRequestHeader = wrappedSet;
    xhrPrototype.send = wrappedSend;
  }
  return () => {
    enabled = false;
    if (globalThis.fetch === wrappedFetch) globalThis.fetch = originalFetch;
    if (xhrPrototype && originalOpen && xhrPrototype.open === wrappedOpen) xhrPrototype.open = originalOpen;
    if (xhrPrototype && originalSet && xhrPrototype.setRequestHeader === wrappedSet) xhrPrototype.setRequestHeader = originalSet;
    if (xhrPrototype && originalSend && xhrPrototype.send === wrappedSend) xhrPrototype.send = originalSend;
  };
}
