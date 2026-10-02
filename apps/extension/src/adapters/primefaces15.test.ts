import { describe, expect, it } from 'vitest';
import { PrimeFaces15Adapter } from './primefaces15.ts';

describe('PrimeFaces15Adapter', () => {
  it('preserves the remote command receiver, arguments, and Promise return', async () => {
    const response = { jqXHR: {}, textStatus: 'success' };
    const promise = Promise.resolve(response);
    const config = { s: 'form:save', pa: [{ name: 'id', value: 7 }] };
    const receiver = { marker: 'application' };
    let received: unknown[] = [];
    let receivedThis: unknown;
    const interactions: string[] = [];
    const ab = function (this: unknown, ...args: unknown[]) {
      receivedThis = this;
      received = args;
      return promise;
    };
    const primeFaces = { VERSION: '15.0.0', ab };
    const remoteCommand = function (this: unknown) {
      return primeFaces.ab.call(this, config);
    };
    Object.assign(globalThis, {
      PrimeFaces: primeFaces,
      remoteCommand,
      jQuery: { ajaxPrefilter() {} },
    });

    const adapter = new PrimeFaces15Adapter();
    const uninstall = adapter.install({
      origin: 'https://app.example',
      traceId: 'a'.repeat(32),
      nextSpanId: () => 'b'.repeat(16),
      onAjax() {},
      withInteraction: <T>(source: string, invoke: () => T): T => {
        interactions.push(source);
        return invoke();
      },
    });
    adapter.select('form:save');
    const invokeRemoteCommand = (globalThis as typeof globalThis & { remoteCommand: typeof remoteCommand }).remoteCommand;
    const result = invokeRemoteCommand.call(receiver);

    expect(receivedThis).toBe(receiver);
    expect(received).toEqual([config]);
    expect(interactions).toEqual(['form:save']);
    expect(result).toBe(promise);
    await expect(result).resolves.toBe(response);
    uninstall();
  });

  it('reports experimental support for an unknown version without falling back to PrimeFaces 5', () => {
    const diagnostics: string[] = [];
    const originalAb = () => 'application-result';
    Object.assign(globalThis, {
      PrimeFaces: { VERSION: '16.0.0', ab: originalAb },
      jQuery: { ajaxPrefilter() {} },
    });

    const adapter = new PrimeFaces15Adapter();
    const uninstall = adapter.install({
      origin: 'https://app.example',
      traceId: 'a'.repeat(32),
      nextSpanId: () => 'b'.repeat(16),
      onAjax() {},
      onDiagnostic: (code) => diagnostics.push(code),
    });
    const pf = (globalThis as typeof globalThis & { PrimeFaces: { ab: typeof originalAb; ajax?: unknown } }).PrimeFaces;

    expect(diagnostics).toContain('PRIMEFACES_VERSION_EXPERIMENTAL');
    expect(pf.ab).not.toBe(originalAb);
    expect(pf.ajax).toBeUndefined();
    expect(pf.ab()).toBe('application-result');
    uninstall();
  });

  it('marks an unresolved build version as experimental', () => {
    const diagnostics: string[] = [];
    const originalAb = () => 'application-result';
    Object.assign(globalThis, {
      PrimeFaces: { VERSION: '${project.version}', ab: originalAb },
      jQuery: { ajaxPrefilter() {} },
    });
    const adapter = new PrimeFaces15Adapter();
    const uninstall = adapter.install({
      origin: 'https://app.example',
      traceId: 'a'.repeat(32),
      nextSpanId: () => 'b'.repeat(16),
      onAjax() {},
      onDiagnostic: (code) => diagnostics.push(code),
    });

    expect(diagnostics).toContain('PRIMEFACES_VERSION_EXPERIMENTAL');
    expect((globalThis as typeof globalThis & { PrimeFaces: { ab: typeof originalAb } }).PrimeFaces.ab()).toBe('application-result');
    uninstall();
  });

  it('does not install over the known PrimeFaces 5 adapter target', () => {
    const diagnostics: string[] = [];
    const originalAb = () => 'application-result';
    Object.assign(globalThis, {
      PrimeFaces: { VERSION: '5.3.0', ab: originalAb },
      jQuery: { ajaxPrefilter() {} },
    });
    const adapter = new PrimeFaces15Adapter();
    const uninstall = adapter.install({
      origin: 'https://app.example',
      traceId: 'a'.repeat(32),
      nextSpanId: () => 'b'.repeat(16),
      onAjax() {},
      onDiagnostic: (code) => diagnostics.push(code),
    });

    expect(diagnostics).toContain('UNSUPPORTED_PRIMEFACES_VERSION');
    expect((globalThis as typeof globalThis & { PrimeFaces: { ab: typeof originalAb } }).PrimeFaces.ab).toBe(originalAb);
    uninstall();
  });
});
