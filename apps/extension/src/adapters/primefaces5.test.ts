import { describe, expect, it } from 'vitest';
import { PrimeFacesAdapter } from './primefaces5.ts';

describe('PrimeFacesAdapter', () => {
  it('does not link Ajax without the selected PrimeFaces invocation', () => {
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
    adapter.observeAjax({ source: 'form:save', url: 'https://app.example/view', headers: {}, propagation: 'unlinked' });
    adapter.observeAjax({ source: 'poller', url: 'https://app.example/view', headers: {}, propagation: 'unlinked' });
    expect(calls[0]?.traceparent).toBeUndefined();
    expect(calls[1]?.traceparent).toBeUndefined();
    uninstall();
  });

  it('preserves existing headers and does not link another origin', () => {
    Object.assign(globalThis, { PrimeFaces: { ajax: { Request: { handle() {} } } }, jQuery: { ajaxPrefilter() {} } });
    const adapter = new PrimeFacesAdapter();
    adapter.install({ origin: 'https://app.example', traceId: 'a'.repeat(32), nextSpanId: () => 'b'.repeat(16), onAjax() {} });
    adapter.select('form:save');
    const other = adapter.observeAjax({ source: 'form:save', url: 'https://other.example/view', headers: { 'X-App': 'kept' }, propagation: 'unlinked' });
    const same = adapter.observeAjax({ source: 'form:save', url: 'https://app.example/view', headers: { 'X-App': 'kept' }, propagation: 'unlinked' });
    expect(other.headers).toEqual({ 'X-App': 'kept' });
    expect(same.headers).toEqual({ 'X-App': 'kept' });
  });

  it('does not link later polling even when it reports the selected source', async () => {
    Object.assign(globalThis, { PrimeFaces: { ajax: { Request: { handle() {} } } }, jQuery: { ajaxPrefilter() {} } });
    const adapter = new PrimeFacesAdapter();
    adapter.install({ origin: 'https://app.example', traceId: 'a'.repeat(32), nextSpanId: () => 'b'.repeat(16), onAjax() {} });
    adapter.select('form:save');
    await Promise.resolve();
    const polling = adapter.observeAjax({ source: 'form:save', url: 'https://app.example/view', headers: {}, propagation: 'unlinked' });
    expect(polling.traceparent).toBeUndefined();
  });

  it('leaves a jQuery transport outside the PrimeFaces invocation unchanged', () => {
    let prefilter: ((options: { url?: string; data?: unknown; headers?: Record<string, string> }) => void) | undefined;
    const handle = function (this: object, value: string) { return value; };
    Object.assign(globalThis, { PrimeFaces: { ajax: { Request: { handle } } }, jQuery: { ajaxPrefilter(callback: typeof prefilter) { prefilter = callback; } } });
    const adapter = new PrimeFacesAdapter();
    adapter.install({ origin: 'https://app.example', traceId: 'a'.repeat(32), nextSpanId: () => 'b'.repeat(16), onAjax() {} });
    adapter.select('form:save');
    const options = { url: 'https://app.example/view', data: 'javax.faces.source=form%3Asave', headers: { 'X-App': 'kept' } };
    prefilter!(options);
    expect(options.headers).toEqual({ 'X-App': 'kept' });
    expect((globalThis as typeof globalThis & { PrimeFaces: { ajax: { Request: { handle: typeof handle } } } }).PrimeFaces.ajax.Request.handle.call({}, 'return')).toBe('return');
  });

  it('links only the Ajax transport nested in the selected PrimeFaces invocation', () => {
    let prefilter: ((options: { url?: string; data?: unknown; headers?: Record<string, string>; xhr?: () => FakeXHR }) => void) | undefined;
    const calls: Array<{ source?: string; propagation?: string; spanId?: string }> = [];
    class FakeXHR {
      headers: Record<string, string> = {};
      setRequestHeader(name: string, value: string) { this.headers[name.toLowerCase()] = value; }
      send() { return 'sent'; }
    }
    const ajax = (source: string) => {
      const options = { url: 'https://app.example/view', data: `javax.faces.source=${encodeURIComponent(source)}`, headers: { 'X-App': 'kept' } as Record<string, string>, xhr: () => new FakeXHR() };
      prefilter!(options);
      const xhr = options.xhr();
      for (const [name, value] of Object.entries(options.headers)) xhr.setRequestHeader(name, value);
      xhr.send();
      return { options, xhr };
    };
    const handle = function (this: { marker: string }, cfg: { source: string }) {
      expect(this.marker).toBe('application');
      const selected = ajax(cfg.source);
      return { selected, value: 'application-result' };
    };
    Object.assign(globalThis, { PrimeFaces: { ajax: { Request: { handle } } }, jQuery: { ajaxPrefilter(callback: typeof prefilter) { prefilter = callback; } } });
    const adapter = new PrimeFacesAdapter();
    const uninstall = adapter.install({ origin: 'https://app.example', traceId: 'a'.repeat(32), nextSpanId: () => 'b'.repeat(16), onAjax: (call) => calls.push(call) });
    const button = new EventTarget();
    let polling: ReturnType<typeof ajax> | undefined;
    let result: ReturnType<typeof handle> | undefined;
    button.addEventListener('click', () => adapter.select('form:save'));
    button.addEventListener('click', () => {
      polling = ajax('form:save');
      result = (globalThis as typeof globalThis & { PrimeFaces: { ajax: { Request: { handle: typeof handle } } } }).PrimeFaces.ajax.Request.handle.call({ marker: 'application' }, { source: 'form:save' });
    });
    button.dispatchEvent(new Event('click'));
    expect(polling?.xhr.headers.traceparent).toBeUndefined();
    expect(result?.value).toBe('application-result');
    expect(result?.selected.xhr.headers.traceparent).toBe('00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01');
    expect(calls.filter((call) => call.propagation === 'propagated')).toEqual([expect.objectContaining({ source: 'form:save', spanId: 'b'.repeat(16) })]);
    uninstall();
  });

  it('records an attempt without exposing a span when a later transport header changes it', () => {
    let prefilter: ((options: { url?: string; data?: unknown; headers?: Record<string, string>; xhr?: () => FakeXHR }) => void) | undefined;
    const calls: Array<{ propagation?: string; spanId?: string }> = [];
    class FakeXHR {
      headers: Record<string, string> = {};
      setRequestHeader(name: string, value: string) { this.headers[name.toLowerCase()] = value; }
      send() { return 'sent'; }
    }
    const handle = (_cfg: { source: string }) => {
      const options = { url: 'https://app.example/view', data: 'javax.faces.source=form%3Asave', headers: {} as Record<string, string>, xhr: () => new FakeXHR() };
      prefilter!(options);
      const xhr = options.xhr();
      for (const [name, value] of Object.entries(options.headers)) xhr.setRequestHeader(name, value);
      xhr.setRequestHeader('traceparent', '00-cccccccccccccccccccccccccccccccc-dddddddddddddddd-01');
      xhr.send();
    };
    Object.assign(globalThis, { PrimeFaces: { ajax: { Request: { handle } } }, jQuery: { ajaxPrefilter(callback: typeof prefilter) { prefilter = callback; } } });
    const adapter = new PrimeFacesAdapter();
    adapter.install({ origin: 'https://app.example', traceId: 'a'.repeat(32), nextSpanId: () => 'b'.repeat(16), onAjax: (call) => calls.push(call) });
    adapter.select('form:save');
    (globalThis as typeof globalThis & { PrimeFaces: { ajax: { Request: { handle: typeof handle } } } }).PrimeFaces.ajax.Request.handle({ source: 'form:save' });
    expect(calls).toContainEqual(expect.objectContaining({ propagation: 'attempted' }));
    expect(calls.some((call) => call.propagation === 'propagated')).toBe(false);
    expect(calls.every((call) => call.spanId === undefined)).toBe(true);
  });

  it('does not claim propagation when repeated case-insensitive XHR headers combine', () => {
    let prefilter: ((options: { url?: string; data?: unknown; headers?: Record<string, string>; xhr?: () => FakeXHR }) => void) | undefined;
    const calls: Array<{ propagation?: string; spanId?: string }> = [];
    class FakeXHR {
      headers = new Map<string, string>();
      setRequestHeader(name: string, value: string) {
        const key = name.toLowerCase();
        const previous = this.headers.get(key);
        this.headers.set(key, previous === undefined ? value : `${previous}, ${value}`);
      }
      send() { return 'sent'; }
    }
    const handle = (_cfg: { source: string }) => {
      const options = { url: 'https://app.example/view', data: 'javax.faces.source=form%3Asave', headers: {} as Record<string, string>, xhr: () => new FakeXHR() };
      prefilter!(options);
      const xhr = options.xhr();
      for (const [name, value] of Object.entries(options.headers)) xhr.setRequestHeader(name, value);
      xhr.setRequestHeader('TraceParent', options.headers.traceparent!);
      xhr.send();
      return xhr;
    };
    Object.assign(globalThis, { PrimeFaces: { ajax: { Request: { handle } } }, jQuery: { ajaxPrefilter(callback: typeof prefilter) { prefilter = callback; } } });
    const adapter = new PrimeFacesAdapter();
    const uninstall = adapter.install({ origin: 'https://app.example', traceId: 'a'.repeat(32), nextSpanId: () => 'b'.repeat(16), onAjax: (call) => calls.push(call) });
    adapter.select('form:save');
    const xhr = (globalThis as typeof globalThis & { PrimeFaces: { ajax: { Request: { handle: typeof handle } } } }).PrimeFaces.ajax.Request.handle({ source: 'form:save' });
    expect(xhr.headers.get('traceparent')).toContain(', ');
    expect(calls.map((call) => call.propagation)).toEqual(['attempted']);
    expect(calls.every((call) => call.spanId === undefined)).toBe(true);
    uninstall();
  });
});
