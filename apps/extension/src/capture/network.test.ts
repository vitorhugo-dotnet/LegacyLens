import { describe, expect, it, vi } from 'vitest';
import { readFileSync } from 'node:fs';
import { installNetworkCapture, type NetworkEffect } from './network.ts';
import { armInteraction, installAsyncContextCapture, withInteraction } from './async-context.ts';
import type { PageCaptureContext } from '../adapters/primefaces5.ts';

const traceId = 'a'.repeat(32);
function context(effects: NetworkEffect[], span = 'b'.repeat(16)): PageCaptureContext {
  return { origin: 'https://app.example', traceId, nextSpanId: () => span, onAjax() {}, onNetwork: (effect) => effects.push(effect) };
}
const interaction = { source: 'save', origin: 'https://app.example', traceId };

describe('network capture', () => {
  it('keeps fetch result and application headers while exposing only an observed same-origin span', async () => {
    const effects: NetworkEffect[] = [];
    const calls: Request[] = [];
    const original = globalThis.fetch;
    globalThis.fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      calls.push(new Request(input, init));
      return new Response('ok');
    };
    const uninstall = installNetworkCapture(context(effects));
    try {
      const base = new Request('https://app.example/save?token=secret', { headers: { 'X-App': 'kept' } });
      const response = await withInteraction(interaction, () => fetch(base));
      expect(await response.text()).toBe('ok');
      expect(base.headers.get('traceparent')).toBeNull();
      expect(calls[0]?.headers.get('x-app')).toBe('kept');
      expect(calls[0]?.headers.get('traceparent')).toBe(`00-${traceId}-${'b'.repeat(16)}-01`);
      expect(effects).toEqual([expect.objectContaining({ transport: 'fetch', propagation: 'propagated', spanId: 'b'.repeat(16) })]);
      expect(JSON.stringify(effects)).not.toContain('secret');
    } finally { uninstall(); globalThis.fetch = original; }
  });

  it('links an inline fetch in the selected click task but leaves the next task poll unlinked', async () => {
    const effects: NetworkEffect[] = [];
    const calls: Request[] = [];
    const original = globalThis.fetch;
    globalThis.fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      calls.push(new Request(input, init));
      return new Response('ok');
    };
    const stopNetwork = installNetworkCapture(context(effects));
    try {
      armInteraction(interaction);
      await Promise.resolve();
      await fetch('https://app.example/save');
      await new Promise((resolve) => setTimeout(resolve, 0));
      await fetch('https://app.example/poll');
      expect(calls.map((call) => call.headers.has('traceparent'))).toEqual([true, false]);
      expect(effects).toEqual([expect.objectContaining({ source: 'save', transport: 'fetch', propagation: 'propagated' })]);
    } finally { stopNetwork(); globalThis.fetch = original; }
  });

  it('leaves unlinked, cross-origin and existing traceparent requests untouched', async () => {
    const effects: NetworkEffect[] = [];
    const calls: Request[] = [];
    const original = globalThis.fetch;
    globalThis.fetch = async (input: RequestInfo | URL, init?: RequestInit) => { calls.push(new Request(input, init)); return new Response(); };
    const uninstall = installNetworkCapture(context(effects));
    try {
      await fetch('https://app.example/poll');
      await withInteraction(interaction, () => fetch('https://other.example/api'));
      await withInteraction(interaction, () => fetch('https://app.example/api', { headers: { TraceParent: 'application' } }));
      await withInteraction(interaction, () => fetch('https://app.example/pf', { headers: { traceparent: `00-${traceId}-${'c'.repeat(16)}-01` } }));
      expect(calls.map((call) => call.headers.get('traceparent'))).toEqual([null, null, 'application', `00-${traceId}-${'c'.repeat(16)}-01`]);
      expect(effects.every((effect) => effect.spanId === undefined)).toBe(true);
      expect(effects).toHaveLength(1);
    } finally { uninstall(); globalThis.fetch = original; }
  });

  it('preserves fetch rejection and XHR return/exception; repeated XHR headers never claim propagation', async () => {
    const error = new Error('application failure');
    const effects: NetworkEffect[] = [];
    const originalFetch = globalThis.fetch;
    const originalXHR = globalThis.XMLHttpRequest;
    globalThis.fetch = async () => { throw error; };
    class FakeXHR {
      static last?: FakeXHR;
      headers = new Map<string, string>();
      url = '';
      constructor() { FakeXHR.last = this; }
      open(_method: string, url: string) { this.url = url; }
      setRequestHeader(name: string, value: string) { const key = name.toLowerCase(); this.headers.set(key, this.headers.has(key) ? `${this.headers.get(key)}, ${value}` : value); }
      send() { return 'sent'; }
    }
    globalThis.XMLHttpRequest = FakeXHR as unknown as typeof XMLHttpRequest;
    const uninstall = installNetworkCapture(context(effects));
    try {
      await expect(withInteraction(interaction, () => fetch('https://app.example/api'))).rejects.toBe(error);
      const xhr = new XMLHttpRequest();
      xhr.open('POST', 'https://app.example/save');
      xhr.setRequestHeader('X-App', 'kept');
      expect(withInteraction(interaction, () => xhr.send())).toBe('sent');
      expect(FakeXHR.last?.headers.get('traceparent')).toBe(`00-${traceId}-${'b'.repeat(16)}-01`);
      const repeated = new XMLHttpRequest();
      repeated.open('POST', 'https://app.example/save');
      repeated.setRequestHeader('TraceParent', 'application');
      withInteraction(interaction, () => repeated.send());
      expect(FakeXHR.last?.headers.get('traceparent')).toBe('application');
      expect(effects.some((effect) => effect.transport === 'xhr' && effect.propagation === 'propagated')).toBe(true);
    } finally { uninstall(); globalThis.fetch = originalFetch; globalThis.XMLHttpRequest = originalXHR; }
  });

  it('does not make an application XHR fail when the new header is rejected', () => {
    const effects: NetworkEffect[] = [];
    const originalXHR = globalThis.XMLHttpRequest;
    class FakeXHR {
      open() {}
      setRequestHeader(name: string) { if (name.toLowerCase() === 'traceparent') throw new Error('header rejected'); }
      send() { return 'application-result'; }
    }
    globalThis.XMLHttpRequest = FakeXHR as unknown as typeof XMLHttpRequest;
    const uninstall = installNetworkCapture(context(effects));
    try {
      const xhr = new XMLHttpRequest();
      xhr.open('GET', 'https://app.example/api');
      expect(withInteraction(interaction, () => xhr.send())).toBe('application-result');
      expect(effects.every((effect) => effect.spanId === undefined)).toBe(true);
    } finally { uninstall(); globalThis.XMLHttpRequest = originalXHR; }
  });

  it('restores fetch and XHR methods on uninstall', () => {
    const originalFetch = globalThis.fetch;
    const originalXHR = globalThis.XMLHttpRequest;
    class FakeXHR { open() {} setRequestHeader() {} send() {} }
    globalThis.XMLHttpRequest = FakeXHR as unknown as typeof XMLHttpRequest;
    const open = FakeXHR.prototype.open;
    const set = FakeXHR.prototype.setRequestHeader;
    const send = FakeXHR.prototype.send;
    const uninstall = installNetworkCapture(context([]));
    uninstall();
    expect(globalThis.fetch).toBe(originalFetch);
    expect(FakeXHR.prototype.open).toBe(open);
    expect(FakeXHR.prototype.setRequestHeader).toBe(set);
    expect(FakeXHR.prototype.send).toBe(send);
    globalThis.XMLHttpRequest = originalXHR;
  });

  it('does not reuse context from XHR open for an unrelated send, and uses B at B send', () => {
    const effects: NetworkEffect[] = [];
    const originalXHR = globalThis.XMLHttpRequest;
    class FakeXHR {
      headers = new Map<string, string>();
      open() {}
      setRequestHeader(name: string, value: string) { this.headers.set(name.toLowerCase(), value); }
      send() { return 'sent'; }
    }
    globalThis.XMLHttpRequest = FakeXHR as unknown as typeof XMLHttpRequest;
    const uninstall = installNetworkCapture(context(effects));
    try {
      const polling = new XMLHttpRequest() as unknown as FakeXHR & XMLHttpRequest;
      withInteraction(interaction, () => polling.open('GET', 'https://app.example/poll'));
      expect(polling.send()).toBe('sent');
      expect(polling.headers.has('traceparent')).toBe(false);
      expect(effects).toEqual([]);
      const laterB = new XMLHttpRequest() as unknown as FakeXHR & XMLHttpRequest;
      withInteraction(interaction, () => laterB.open('GET', 'https://app.example/next'));
      withInteraction({ ...interaction, source: 'B' }, () => laterB.send());
      expect(laterB.headers.get('traceparent')).toBe(`00-${traceId}-${'b'.repeat(16)}-01`);
      expect(effects).toEqual([expect.objectContaining({ source: 'B', propagation: 'propagated' })]);
    } finally { uninstall(); globalThis.XMLHttpRequest = originalXHR; }
  });

  it('marks a timer-to-fetch stack as an observed partial async chain', () => {
    vi.useFakeTimers();
    const effects: NetworkEffect[] = [];
    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => new Response();
    const stopAsync = installAsyncContextCapture();
    const stopNetwork = installNetworkCapture(context(effects));
    try {
      function second() { void fetch('https://app.example/save'); }
      function first() { setTimeout(second, 0); }
      withInteraction(interaction, first);
      vi.runAllTimers();
      expect(effects).toEqual([expect.objectContaining({ source: 'save', stackGap: 'ASYNC_BOUNDARY' })]);
    } finally { stopNetwork(); stopAsync(); globalThis.fetch = originalFetch; vi.useRealTimers(); }
  });

  it('runs the static browser fixture with selected A, unlinked B and polling, retaining an async gap', async () => {
    vi.useFakeTimers();
    const previousElement = globalThis.Element;
    const previousDocument = globalThis.document;
    const previousFetch = globalThis.fetch;
    class FakeElement extends EventTarget { textContent = ''; constructor(public id: string) { super(); } closest() { return { id: this.id }; } }
    const save = new FakeElement('save');
    const other = new FakeElement('other');
    const result = new FakeElement('result');
    globalThis.Element = FakeElement as unknown as typeof Element;
    globalThis.document = { querySelector: (selector: string) => selector === '#save' ? save : selector === '#other' ? other : result } as unknown as Document;
    const requests: Request[] = [];
    globalThis.fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      requests.push(new Request(input instanceof Request ? input : new URL(String(input), 'https://app.example'), init));
      return new Response();
    };
    const effects: NetworkEffect[] = [];
    const stopAsync = installAsyncContextCapture();
    try {
      const html = readFileSync(new URL('../../../../fixtures/static/browser/interaction.html', import.meta.url), 'utf8');
      const script = html.match(/<script>([\s\S]*?)<\/script>/)?.[1];
      expect(script).toBeDefined();
      new Function(script!)();
      const stopNetwork = installNetworkCapture(context(effects));
      try {
        armInteraction(interaction);
        save.dispatchEvent(new Event('click'));
        vi.advanceTimersByTime(0);
        await Promise.resolve();
        other.dispatchEvent(new Event('click'));
        vi.advanceTimersByTime(0);
        vi.advanceTimersByTime(5000);
        await Promise.resolve();
        expect(requests.map((request) => request.headers.has('traceparent'))).toEqual([true, false, false]);
        expect(effects).toEqual([expect.objectContaining({ source: 'save', stackGap: 'ASYNC_BOUNDARY' })]);
      } finally { stopNetwork(); }
    } finally {
      stopAsync(); globalThis.Element = previousElement; globalThis.document = previousDocument; globalThis.fetch = previousFetch; vi.useRealTimers();
    }
  });
});
