import { describe, expect, it } from 'vitest';
import { installNetworkCapture, type NetworkEffect } from './network.ts';
import { withInteraction } from './async-context.ts';
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
});
