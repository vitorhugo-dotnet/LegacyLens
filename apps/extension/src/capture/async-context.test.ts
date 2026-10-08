import { afterEach, describe, expect, it, vi } from 'vitest';
import { armInteraction, bindInteraction, currentInteraction, installAsyncContextCapture, withInteraction, type InteractionContext } from './async-context.ts';

const interaction = (source: string): InteractionContext => ({ source, origin: 'https://app.example', traceId: 'a'.repeat(32) });

describe('async interaction context', () => {
  afterEach(() => vi.useRealTimers());

  it('binds timer and Promise callbacks at registration, without inheriting polling or the next click', async () => {
    vi.useFakeTimers();
    const uninstall = installAsyncContextCapture();
    try {
      const seen: string[] = [];
      setTimeout(() => seen.push(currentInteraction()?.source ?? 'unlinked'), 1);
      withInteraction(interaction('A'), () => {
        setTimeout(() => seen.push(currentInteraction()?.source ?? 'unlinked'), 1);
        Promise.resolve().then(() => seen.push(currentInteraction()?.source ?? 'unlinked'));
      });
      withInteraction(interaction('B'), () => setTimeout(() => seen.push(currentInteraction()?.source ?? 'unlinked'), 1));
      await Promise.resolve();
      vi.runAllTimers();
      expect(seen).toEqual(['A', 'unlinked', 'A', 'B']);
    } finally { uninstall(); }
  });

  it('preserves receiver, arguments, result and exception', () => {
    const receiver = { value: 4 };
    const wrapped = bindInteraction(function (this: typeof receiver, n: number) {
      expect(currentInteraction()?.source).toBe('A');
      return this.value + n;
    }, interaction('A'));
    expect(wrapped.call(receiver, 3)).toBe(7);
    const error = new Error('application');
    expect(() => bindInteraction(() => { throw error; }, interaction('A'))()).toThrow(error);
    expect(currentInteraction()).toBeUndefined();
  });

  it('arms only the selected click listener and does not link a second click', async () => {
    const previousElement = globalThis.Element;
    class FakeElement extends EventTarget { id = 'save'; closest() { return { id: this.id }; } }
    globalThis.Element = FakeElement as unknown as typeof Element;
    const uninstall = installAsyncContextCapture();
    try {
      const button = new FakeElement();
      const seen: string[] = [];
      button.addEventListener('click', () => seen.push(currentInteraction()?.source ?? 'unlinked'));
      armInteraction(interaction('save'));
      button.dispatchEvent(new Event('click'));
      await new Promise((resolve) => setTimeout(resolve, 0));
      button.dispatchEvent(new Event('click'));
      expect(seen).toEqual(['save', 'unlinked']);
    } finally { uninstall(); globalThis.Element = previousElement; }
  });

  it('keeps the selected click armed until its matching click task arrives', () => {
    vi.useFakeTimers();
    const previousElement = globalThis.Element;
    class FakeElement extends EventTarget { id = 'save'; closest() { return { id: this.id }; } }
    globalThis.Element = FakeElement as unknown as typeof Element;
    const uninstall = installAsyncContextCapture();
    try {
      const button = new FakeElement();
      const seen: string[] = [];
      button.addEventListener('click', () => seen.push(currentInteraction()?.source ?? 'unlinked'));
      armInteraction(interaction('save'));
      vi.advanceTimersByTime(0);
      button.dispatchEvent(new Event('click'));
      vi.advanceTimersByTime(0);
      button.dispatchEvent(new Event('click'));
      expect(seen).toEqual(['save', 'unlinked']);
    } finally { uninstall(); globalThis.Element = previousElement; }
  });

  it('keeps the selected click armed across a microtask checkpoint during the same click task', async () => {
    const previousElement = globalThis.Element;
    class FakeElement extends EventTarget { id = 'save'; closest() { return { id: this.id }; } }
    globalThis.Element = FakeElement as unknown as typeof Element;
    const uninstall = installAsyncContextCapture();
    try {
      const button = new FakeElement();
      let observed = '';
      button.addEventListener('click', () => { observed = currentInteraction()?.source ?? 'unlinked'; });
      armInteraction(interaction('save'));
      await Promise.resolve();
      button.dispatchEvent(new Event('click'));
      expect(observed).toBe('save');
    } finally { uninstall(); globalThis.Element = previousElement; }
  });

  it('uses the triggering click B for a listener registered during A', async () => {
    const previousElement = globalThis.Element;
    class FakeElement extends EventTarget { id = 'save'; closest() { return { id: this.id }; } }
    globalThis.Element = FakeElement as unknown as typeof Element;
    const uninstall = installAsyncContextCapture();
    try {
      const button = new FakeElement();
      const seen: string[] = [];
      const listener = () => seen.push(currentInteraction()?.source ?? 'unlinked');
      withInteraction(interaction('A'), () => button.addEventListener('click', listener));
      button.id = 'B';
      armInteraction(interaction('B'));
      button.dispatchEvent(new Event('click'));
      await new Promise((resolve) => setTimeout(resolve, 0));
      button.dispatchEvent(new Event('click'));
      button.removeEventListener('click', listener);
      expect(seen).toEqual(['B', 'unlinked']);
    } finally { uninstall(); globalThis.Element = previousElement; }
  });

  it('keeps listener identity for removal, capture options, once and abort', () => {
    const uninstall = installAsyncContextCapture();
    try {
      const target = new EventTarget();
      const seen: string[] = [];
      const listener = () => seen.push(currentInteraction()?.source ?? 'unlinked');
      withInteraction(interaction('A'), () => target.addEventListener('save', listener, { capture: true }));
      target.dispatchEvent(new Event('save'));
      withInteraction(interaction('A'), () => target.dispatchEvent(new Event('save')));
      target.removeEventListener('save', listener, { capture: true });
      target.dispatchEvent(new Event('save'));
      const abort = new AbortController();
      withInteraction(interaction('B'), () => target.addEventListener('save', listener, { once: true, signal: abort.signal }));
      withInteraction(interaction('B'), () => target.dispatchEvent(new Event('save')));
      target.dispatchEvent(new Event('save'));
      withInteraction(interaction('B'), () => target.addEventListener('save', listener, { once: true }));
      withInteraction(interaction('B'), () => target.dispatchEvent(new Event('save')));
      abort.abort();
      expect(seen).toEqual(['unlinked', 'A', 'B', 'B']);
    } finally { uninstall(); }
  });

  it('restores APIs and leaves previously registered application listeners removable', () => {
    const originalAdd = EventTarget.prototype.addEventListener;
    const originalRemove = EventTarget.prototype.removeEventListener;
    const originalThen = Promise.prototype.then;
    const target = new EventTarget();
    let calls = 0;
    const listener = () => { calls++; };
    const uninstall = installAsyncContextCapture();
    withInteraction(interaction('A'), () => target.addEventListener('save', listener));
    uninstall();
    expect(EventTarget.prototype.addEventListener).toBe(originalAdd);
    expect(EventTarget.prototype.removeEventListener).toBe(originalRemove);
    expect(Promise.prototype.then).toBe(originalThen);
    target.dispatchEvent(new Event('save'));
    target.removeEventListener('save', listener);
    target.dispatchEvent(new Event('save'));
    expect(calls).toBe(1);
  });
});
