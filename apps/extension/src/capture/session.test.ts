import { describe, expect, it } from 'vitest';
import { CaptureController } from './session.ts';
import { NativeRequestError } from '../native/client.ts';

describe('CaptureController', () => {
  it('correlates host ingest acceptance and safe failures with the click identity', async () => {
    const accepted: unknown[] = [];
    const successful = new CaptureController({ request: async <T>(command: string): Promise<T> => command === 'capture.start'
      ? { id: 'a'.repeat(32), projectId: 'project', expiresAt: new Date(Date.now() + 60000).toISOString() } as T : {} as T
    }, undefined, (record) => accepted.push(record));
    await successful.start({ projectId: 'project', tabId: 19, origin: 'https://app.example' });
    await successful.record(19, 'jsf.click', { source: 'sensitive-form-name' }, { eventId: 'b'.repeat(16) });
    expect(accepted).toHaveLength(2);
    expect(accepted[0]).toEqual({ stage: 'host.ingest', outcome: 'started', traceId: 'a'.repeat(32), tabId: 19, eventId: 'b'.repeat(16) });
    expect(accepted[1]).toMatchObject({ stage: 'host.ingest', outcome: 'accepted', traceId: 'a'.repeat(32), tabId: 19,
      eventId: 'b'.repeat(16), durationMs: expect.any(Number) });
    expect(JSON.stringify(accepted)).not.toContain('sensitive-form-name');

    const timedOut: unknown[] = [];
    const failed = new CaptureController({ request: async <T>(command: string): Promise<T> => {
      if (command === 'capture.start') return { id: 'c'.repeat(32), projectId: 'project', expiresAt: new Date(Date.now() + 60000).toISOString() } as T;
      throw new NativeRequestError('timeout', 'private native detail');
    } }, undefined, (record) => timedOut.push(record));
    await failed.start({ projectId: 'project', tabId: 20, origin: 'https://app.example' });
    await expect(failed.record(20, 'jsf.click', { source: 'another-secret' }, { eventId: 'd'.repeat(16) })).rejects.toThrow('transport interrupted');
    expect(timedOut.at(-1)).toEqual({ stage: 'host.ingest', outcome: 'timeout', traceId: 'c'.repeat(32), tabId: 20,
      eventId: 'd'.repeat(16), durationMs: expect.any(Number), code: 'TIMEOUT' });
    expect(JSON.stringify(timedOut)).not.toContain('private native detail');
    expect(JSON.stringify(timedOut)).not.toContain('another-secret');
  });
  it('writes the selected click as the network event parent in the ingested protocol', async () => {
    const events: Array<{ eventId: string; parentEventId?: string; kind: string }> = [];
    const controller = new CaptureController({ request: async <T>(command: string, payload: unknown): Promise<T> => {
      if (command === 'capture.start') return { id: 'a'.repeat(32), projectId: 'project', startedAt: new Date().toISOString(), expiresAt: new Date(Date.now() + 60000).toISOString() } as T;
      if (command === 'trace.ingest') events.push(...(payload as { events: typeof events }).events);
      return {} as T;
    } });
    await controller.start({ projectId: 'project', tabId: 8, origin: 'https://app.example' });
    await controller.record(8, 'jsf.click', { source: 'save' }, { eventId: '1'.repeat(16) });
    await controller.record(8, 'browser.network', { source: 'save', spanId: '2'.repeat(16) }, { parentEventId: '1'.repeat(16) });
    expect(events).toMatchObject([{ eventId: '1'.repeat(16), kind: 'jsf.click' },
      { parentEventId: '1'.repeat(16), kind: 'browser.network' }]);
  });

  it('returns host acceptance identity including duplicate replay status', async () => {
    let duplicate = 0;
    const controller = new CaptureController({ request: async <T>(command: string): Promise<T> => {
      if (command === 'capture.start') return { id: 'a'.repeat(32), projectId: 'project', expiresAt: new Date(Date.now() + 60000).toISOString() } as T;
      return { accepted: duplicate ? 0 : 1, duplicate } as T;
    } });
    await controller.start({ projectId: 'project', tabId: 18, origin: 'https://app.example' });
    await expect(controller.record(18, 'jsf.click', {}, { eventId: '1'.repeat(16) }))
      .resolves.toEqual({ gap: false, duplicate: false });
    duplicate = 1;
    await expect(controller.record(18, 'jsf.click', {}, { eventId: '1'.repeat(16) }))
      .resolves.toEqual({ gap: false, duplicate: true });
  });
  it('keeps tabs isolated and permits only one capture per tab', async () => {
    const starts: string[] = [];
    const controller = new CaptureController({
      request: async <T>(_command: string, payload: unknown): Promise<T> => {
        const tabId = String((payload as { tabId: string }).tabId);
        starts.push(tabId);
        return { id: tabId.repeat(32), projectId: 'project', startedAt: new Date().toISOString(), expiresAt: new Date(Date.now() + 60000).toISOString() } as T;
      },
    });
    await controller.start({ projectId: 'project', tabId: 1, origin: 'https://app.example' });
    await expect(controller.start({ projectId: 'project', tabId: 1, origin: 'https://app.example' })).rejects.toThrow();
    await controller.start({ projectId: 'project', tabId: 2, origin: 'https://app.example' });
    expect(starts).toEqual(['1', '2']);
  });

  it('does not start two captures concurrently in the same tab', async () => {
    let calls = 0;
    const controller = new CaptureController({ request: async <T>(): Promise<T> => {
      calls++;
      await new Promise((resolve) => setTimeout(resolve, 1));
      return { id: 'a'.repeat(32), projectId: 'project', startedAt: new Date().toISOString(), expiresAt: new Date(Date.now() + 60000).toISOString() } as T;
    } });
    const result = await Promise.allSettled([
      controller.start({ projectId: 'project', tabId: 7, origin: 'https://app.example' }),
      controller.start({ projectId: 'project', tabId: 7, origin: 'https://app.example' }),
    ]);
    expect(result.filter((item) => item.status === 'fulfilled')).toHaveLength(1);
    expect(calls).toBe(1);
  });

  it('reports a recovery gap before the next event after background suspension', async () => {
    const commands: Array<{ command: string; payload: unknown }> = [];
    const stored = [{ request: { projectId: 'project', tabId: 3, origin: 'https://app.example' },
      session: { id: 'a'.repeat(32), projectId: 'project', startedAt: new Date().toISOString(), expiresAt: new Date(Date.now() + 60000).toISOString() },
      producerId: 'producer', sequence: 2, gap: false }];
    const controller = new CaptureController({ request: async <T>(command: string, payload: unknown): Promise<T> => {
      commands.push({ command, payload });
      return { accepted: 1, duplicate: 0, diagnostics: [] } as T;
    } }, { load: async () => stored, save: async () => {} });
    await controller.record(3, 'jsf.click', { source: 'form:save' });
    const kinds = commands.map(({ payload }) => (payload as { events: Array<{ kind: string }> }).events[0]?.kind);
    expect(kinds).toEqual(['extension.gap', 'jsf.click']);
  });

  it('finishes pending ingestion before stop and rejects events queued after stop', async () => {
    const calls: string[] = [];
    let release!: () => void;
    const pending = new Promise<void>((resolve) => { release = resolve; });
    const controller = new CaptureController({ request: async <T>(command: string): Promise<T> => {
      calls.push(command);
      if (command === 'trace.ingest') await pending;
      if (command === 'capture.start') return { id: 'a'.repeat(32), projectId: 'project', startedAt: new Date().toISOString(), expiresAt: new Date(Date.now() + 60000).toISOString() } as T;
      return {} as T;
    } });
    await controller.start({ projectId: 'project', tabId: 4, origin: 'https://app.example' });
    const record = controller.record(4, 'jsf.click', { source: 'form:save' });
    const stop = controller.stop(4);
    await Promise.resolve();
    expect(calls).not.toContain('capture.stop');
    release();
    await Promise.all([record, stop]);
    await expect(controller.record(4, 'jsf.click', { source: 'later' })).rejects.toThrow('missing or expired');
    expect(calls).toEqual(['capture.start', 'trace.ingest', 'capture.stop']);
  });

  it('rolls back a native capture when saving the new session fails', async () => {
    const commands: string[] = [];
    let saves = 0;
    const controller = new CaptureController({ request: async <T>(command: string): Promise<T> => {
      commands.push(command);
      if (command === 'capture.start') return { id: 'a'.repeat(32), projectId: 'project', startedAt: new Date().toISOString(), expiresAt: new Date(Date.now() + 60000).toISOString() } as T;
      return {} as T;
    } }, { load: async () => [], save: async () => { if (++saves === 1) throw new Error('storage unavailable'); } });
    await expect(controller.start({ projectId: 'project', tabId: 5, origin: 'https://app.example' })).rejects.toThrow('could not be saved');
    expect(commands).toEqual(['capture.start', 'capture.stop']);
    expect(controller.get(5)).toBeUndefined();
    await expect(controller.start({ projectId: 'project', tabId: 5, origin: 'https://app.example' })).resolves.toMatchObject({ id: 'a'.repeat(32) });
  });

  it('keeps a failed native rollback visible and allows a later stop', async () => {
    const commands: string[] = [];
    let stops = 0;
    const controller = new CaptureController({ request: async <T>(command: string): Promise<T> => {
      commands.push(command);
      if (command === 'capture.start') return { id: 'a'.repeat(32), projectId: 'project', startedAt: new Date().toISOString(), expiresAt: new Date(Date.now() + 60000).toISOString() } as T;
      if (command === 'capture.stop' && ++stops === 1) throw new Error('host unavailable');
      return {} as T;
    } }, { load: async () => [], save: async () => { if (stops === 0) throw new Error('storage unavailable'); } });
    await expect(controller.start({ projectId: 'project', tabId: 6, origin: 'https://app.example' })).rejects.toThrow('capture remains active');
    expect(controller.get(6)).toBeDefined();
    await controller.stop(6);
    expect(controller.get(6)).toBeUndefined();
    expect(commands).toEqual(['capture.start', 'capture.stop', 'capture.stop']);
  });
});
