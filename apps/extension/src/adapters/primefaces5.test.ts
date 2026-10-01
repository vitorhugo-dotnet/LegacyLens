import { describe, expect, it } from 'vitest';
import { PrimeFacesAdapter } from './primefaces5.ts';

describe('PrimeFacesAdapter', () => {
  it('links a selected JSF click to its Ajax action but leaves concurrent polling unlinked', () => {
    Object.assign(globalThis, { PrimeFaces: { ajax: { Request: { handle() {} } } }, jQuery: { ajaxPrefilter() {} } });
    const calls: Array<{ source?: string; traceparent?: string }> = [];
    const adapter = new PrimeFacesAdapter();
    const uninstall = adapter.install({
      origin: 'https://app.example',
      traceId: '0123456789abcdef0123456789abcdef',
      nextSpanId: () => '1234567890abcdef',
      onAjax: (call) => calls.push(call),
    });
    adapter.select('form:save');
    adapter.observeAjax({ source: 'form:save', url: 'https://app.example/view', headers: {} });
    adapter.observeAjax({ source: 'poller', url: 'https://app.example/view', headers: {} });
    expect(calls[0]).toMatchObject({ source: 'form:save', traceparent: '00-0123456789abcdef0123456789abcdef-1234567890abcdef-01' });
    expect(calls[1]?.traceparent).toBeUndefined();
    uninstall();
  });

  it('preserves existing headers and does not link another origin', () => {
    Object.assign(globalThis, { PrimeFaces: { ajax: { Request: { handle() {} } } }, jQuery: { ajaxPrefilter() {} } });
    const adapter = new PrimeFacesAdapter();
    adapter.install({ origin: 'https://app.example', traceId: 'a'.repeat(32), nextSpanId: () => 'b'.repeat(16), onAjax() {} });
    adapter.select('form:save');
    const other = adapter.observeAjax({ source: 'form:save', url: 'https://other.example/view', headers: { 'X-App': 'kept' } });
    const same = adapter.observeAjax({ source: 'form:save', url: 'https://app.example/view', headers: { 'X-App': 'kept' } });
    expect(other.headers).toEqual({ 'X-App': 'kept' });
    expect(same.headers).toMatchObject({ 'X-App': 'kept', traceparent: '00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01' });
  });

  it('does not link later polling even when it reports the selected source', async () => {
    Object.assign(globalThis, { PrimeFaces: { ajax: { Request: { handle() {} } } }, jQuery: { ajaxPrefilter() {} } });
    const adapter = new PrimeFacesAdapter();
    adapter.install({ origin: 'https://app.example', traceId: 'a'.repeat(32), nextSpanId: () => 'b'.repeat(16), onAjax() {} });
    adapter.select('form:save');
    await Promise.resolve();
    const polling = adapter.observeAjax({ source: 'form:save', url: 'https://app.example/view', headers: {} });
    expect(polling.traceparent).toBeUndefined();
  });

  it('adds a header through the PrimeFaces jQuery transport without replacing application headers', () => {
    let prefilter: ((options: { url?: string; data?: unknown; headers?: Record<string, string> }) => void) | undefined;
    const handle = function (this: object, value: string) { return value; };
    Object.assign(globalThis, { PrimeFaces: { ajax: { Request: { handle } } }, jQuery: { ajaxPrefilter(callback: typeof prefilter) { prefilter = callback; } } });
    const adapter = new PrimeFacesAdapter();
    adapter.install({ origin: 'https://app.example', traceId: 'a'.repeat(32), nextSpanId: () => 'b'.repeat(16), onAjax() {} });
    adapter.select('form:save');
    const options = { url: 'https://app.example/view', data: 'javax.faces.source=form%3Asave', headers: { 'X-App': 'kept' } };
    prefilter!(options);
    expect(options.headers).toMatchObject({ 'X-App': 'kept', traceparent: '00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01' });
    expect((globalThis as typeof globalThis & { PrimeFaces: { ajax: { Request: { handle: typeof handle } } } }).PrimeFaces.ajax.Request.handle.call({}, 'return')).toBe('return');
  });
});
