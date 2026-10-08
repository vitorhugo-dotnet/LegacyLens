import type { Page, Worker } from '@playwright/test';

interface CaptureReply { ok?: boolean; session?: { id?: string }; error?: string }

async function sendFixtureCommand(worker: Worker, page: Page, message: { type: string; projectId?: string }): Promise<CaptureReply> {
  worker.on('console', (entry) => console.log(`[extension] ${entry.text()}`));
  return await worker.evaluate(async ({ url, message: command }) => {
    const tab = (await chrome.tabs.query({})).find((item) => item.url === url);
    if (tab?.id === undefined) throw new Error('fixture tab is unavailable');
    return await chrome.tabs.sendMessage(tab.id, command) as CaptureReply;
  }, { url: page.url(), message });
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
