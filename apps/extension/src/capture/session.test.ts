import { describe, expect, it } from 'vitest';
import { CaptureController } from './session.ts';

describe('CaptureController', () => {
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
    await controller.record(4, 'jsf.click', { source: 'later' });
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
