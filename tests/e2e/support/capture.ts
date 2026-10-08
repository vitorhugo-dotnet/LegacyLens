import type { Page, Worker } from '@playwright/test';

interface CaptureReply { ok?: boolean; session?: { id?: string }; error?: string }
export interface CaptureStageRecord {
  stage: string; outcome: string; traceId: string; tabId: number; eventId?: string; durationMs?: number; code?: string;
}

export async function captureDiagnostics(worker: Worker): Promise<CaptureStageRecord[]> {
  return await worker.evaluate(() => {
    const records = (globalThis as typeof globalThis & { __legacyLensCaptureDiagnostics?: CaptureStageRecord[] }).__legacyLensCaptureDiagnostics;
    const contentRecords = (chrome.storage.local.get('legacylens.captureDiagnostics.v1') as Promise<Record<string, CaptureStageRecord[]>>);
    return contentRecords.then((stored) => [...(records ?? []), ...(stored['legacylens.captureDiagnostics.v1'] ?? [])]);
  });
}

async function sendFixtureCommand(worker: Worker, page: Page, message: { type: string; projectId?: string }): Promise<CaptureReply> {
  return await worker.evaluate(async ({ url, command }) => {
    const tab = (await chrome.tabs.query({})).find((item) => item.url === url);
    if (tab?.id === undefined) throw new Error('fixture tab is unavailable');
    const driver = (globalThis as typeof globalThis & {
      __legacyLensFixtureCapture?: (request: { action: 'start' | 'stop'; tabId: number; projectId?: string }) => Promise<CaptureReply>;
    }).__legacyLensFixtureCapture;
    if (!driver) throw new Error('fixture capture driver is not available in the extension service worker');
    return await driver({
      action: command.type === 'fixture.capture.start' ? 'start' : 'stop',
      tabId: tab.id,
      ...(command.projectId ? { projectId: command.projectId } : {}),
    });
  }, { url: page.url(), command: message });
}

export async function startFixtureCapture(worker: Worker, page: Page, projectId: string): Promise<void> {
  const reply = await sendFixtureCommand(worker, page, { type: 'fixture.capture.start', projectId });
  if (!reply?.ok || !/^[a-f0-9]{32}$/i.test(reply.session?.id ?? '')) {
    throw new Error(reply?.error ?? 'Fixture capture did not return a valid session');
  }
}

export async function stopFixtureCapture(worker: Worker, page: Page): Promise<void> {
  const reply = await sendFixtureCommand(worker, page, { type: 'fixture.capture.stop' });
  if (!reply?.ok) throw new Error(reply?.error ?? 'Fixture capture did not stop');
}
