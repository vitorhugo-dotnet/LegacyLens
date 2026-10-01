import { describe, expect, it } from 'vitest';
import { CaptureController } from './session.ts';

describe('CaptureController', () => {
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
});
