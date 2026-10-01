export interface InteractionContext {
  source: string;
  origin: string;
  traceId: string;
  asyncBoundary?: true;
}

let active: InteractionContext | undefined;
let armed: InteractionContext | undefined;
let observing = 0;
let restoreInline: (() => void) | undefined;

export function currentInteraction(): InteractionContext | undefined { return observing ? undefined : active; }

export function withInteraction<T>(context: InteractionContext, fn: () => T): T {
  const previous = active;
  active = context;
  try { return fn(); } finally { active = previous; }
}

export function withoutInteraction<T>(fn: () => T): T {
  const previous = active;
  active = undefined;
  observing++;
  try { return fn(); } finally { observing--; active = previous; }
}

export function bindInteraction<T extends (...args: any[]) => any>(fn: T, context: InteractionContext): T {
  return function (this: unknown, ...args: Parameters<T>): ReturnType<T> {
    return withInteraction(context, () => Reflect.apply(fn, this, args));
  } as T;
}

/** Arm only the selected click. Callbacks already registered before installation remain an explicit gap. */
export function armInteraction(context: InteractionContext): void {
  restoreInline?.();
  armed = context;
  const element = typeof document === 'undefined' ? null : document.getElementById(context.source);
  const original = element?.onclick;
  if (element && typeof original === 'function') {
    const wrapped = function (this: GlobalEventHandlers, event: MouseEvent) {
      return withInteraction(context, () => Reflect.apply(original, this, [event]));
    };
    element.onclick = wrapped;
    const restore = () => {
      if (element.onclick === wrapped) element.onclick = original;
      if (restoreInline === restore) restoreInline = undefined;
    };
    restoreInline = restore;
  }
  const restore = restoreInline;
  setTimeout(() => { if (armed === context) armed = undefined; restore?.(); }, 0);
}

export function installAsyncContextCapture(): () => void {
  const originalTimeout = globalThis.setTimeout;
  const originalInterval = globalThis.setInterval;
  const originalMicrotask = globalThis.queueMicrotask;
  const originalThen = Promise.prototype.then;
  const originalAdd = EventTarget.prototype.addEventListener;
  const originalRemove = EventTarget.prototype.removeEventListener;
  const originalFrame = globalThis.requestAnimationFrame;
  const records: Array<{ target: EventTarget; type: string; original: EventListenerOrEventListenerObject; wrapped: EventListener; options: boolean | AddEventListenerOptions | undefined; capture: boolean; abort?: EventListener }> = [];
  let enabled = true;

  const callback = <T extends (...args: any[]) => any>(fn: T): T => active && enabled
    ? bindInteraction(fn, { ...active, asyncBoundary: true }) : fn;
  globalThis.setTimeout = ((handler: TimerHandler, timeout?: number, ...args: any[]) =>
    Reflect.apply(originalTimeout, globalThis, [typeof handler === 'function' ? callback(handler as (...args: any[]) => any) : handler, timeout, ...args])) as typeof setTimeout;
  globalThis.setInterval = ((handler: TimerHandler, timeout?: number, ...args: any[]) =>
    Reflect.apply(originalInterval, globalThis, [typeof handler === 'function' ? callback(handler as (...args: any[]) => any) : handler, timeout, ...args])) as typeof setInterval;
  globalThis.queueMicrotask = ((fn: VoidFunction) => originalMicrotask(callback(fn))) as typeof queueMicrotask;
  Promise.prototype.then = function (onFulfilled, onRejected) {
    return Reflect.apply(originalThen, this, [typeof onFulfilled === 'function' ? callback(onFulfilled) : onFulfilled,
      typeof onRejected === 'function' ? callback(onRejected) : onRejected]);
  };
  if (typeof originalFrame === 'function') globalThis.requestAnimationFrame = ((fn: FrameRequestCallback) => originalFrame(callback(fn))) as typeof requestAnimationFrame;

  EventTarget.prototype.addEventListener = function (type, listener, options) {
    if (!listener) return Reflect.apply(originalAdd, this, [type, listener, options]);
    if (typeof options === 'object' && options?.signal?.aborted) return Reflect.apply(originalAdd, this, [type, listener, options]);
    const capture = typeof options === 'boolean' ? options : !!options?.capture;
    if (records.some((item) => item.target === this && item.type === type && item.original === listener && item.capture === capture)) return;
    const wrapped: EventListener = function (this: EventTarget, event: Event) {
      if (typeof options === 'object' && options?.once) {
        const index = records.indexOf(record);
        if (index >= 0) records.splice(index, 1);
        if (record.abort && options.signal) Reflect.apply(originalRemove, options.signal, ['abort', record.abort, false]);
      }
      const selected = enabled && type === 'click' && armed && typeof Element !== 'undefined' && event.target instanceof Element
        && event.target.closest('[id]')?.id === armed.source ? armed : undefined;
      // Registration alone does not make a later, independently dispatched event a child.
      const context = enabled ? selected ?? active : undefined;
      const invoke = () => typeof listener === 'function'
        ? Reflect.apply(listener, this, [event]) : Reflect.apply(listener.handleEvent, listener, [event]);
      return context ? withInteraction(context, invoke) : invoke();
    };
    const record: (typeof records)[number] = { target: this, type, original: listener, wrapped, options, capture };
    records.push(record);
    try {
      const result = Reflect.apply(originalAdd, this, [type, wrapped, options]);
      if (typeof options === 'object' && options?.signal) {
        const onAbort: EventListener = () => { const index = records.indexOf(record); if (index >= 0) records.splice(index, 1); };
        Reflect.apply(originalAdd, options.signal, ['abort', onAbort, { once: true }]);
        record.abort = onAbort;
      }
      return result;
    }
    catch (error) { records.splice(records.indexOf(record), 1); throw error; }
  };
  EventTarget.prototype.removeEventListener = function (type, listener, options) {
    const capture = typeof options === 'boolean' ? options : !!options?.capture;
    const index = records.findIndex((item) => item.target === this && item.type === type && item.original === listener && item.capture === capture);
    const record = index < 0 ? undefined : records.splice(index, 1)[0]!;
    if (record?.abort && typeof record.options === 'object' && record.options.signal)
      Reflect.apply(originalRemove, record.options.signal, ['abort', record.abort, false]);
    const wrapped = record?.wrapped ?? listener;
    return Reflect.apply(originalRemove, this, [type, wrapped, options]);
  };
  return () => {
    enabled = false;
    armed = undefined;
    restoreInline?.();
    if (globalThis.setTimeout !== originalTimeout) globalThis.setTimeout = originalTimeout;
    if (globalThis.setInterval !== originalInterval) globalThis.setInterval = originalInterval;
    if (globalThis.queueMicrotask !== originalMicrotask) globalThis.queueMicrotask = originalMicrotask;
    if (Promise.prototype.then !== originalThen) Promise.prototype.then = originalThen;
    if (typeof originalFrame === 'function') globalThis.requestAnimationFrame = originalFrame;
    if (EventTarget.prototype.addEventListener !== originalAdd) EventTarget.prototype.addEventListener = originalAdd;
    if (EventTarget.prototype.removeEventListener !== originalRemove) EventTarget.prototype.removeEventListener = originalRemove;
    for (const item of records) {
      if (item.abort && typeof item.options === 'object' && item.options.signal)
        Reflect.apply(originalRemove, item.options.signal, ['abort', item.abort, false]);
      Reflect.apply(originalRemove, item.target, [item.type, item.wrapped, item.capture]);
      if (!item.options || typeof item.options !== 'object' || !item.options.signal?.aborted)
        Reflect.apply(originalAdd, item.target, [item.type, item.original, item.options]);
    }
  };
}
