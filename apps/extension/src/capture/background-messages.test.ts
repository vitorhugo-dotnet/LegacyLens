import { describe, expect, it, vi } from 'vitest';
import { handleCaptureEvent } from './background-messages.ts';

const traceId = 'a'.repeat(32);
const eventId = 'b'.repeat(16);
const active = { session: { id: traceId, expiresAt: new Date(Date.now() + 60_000).toISOString() }, request: { origin: 'http://localhost:8180' }, gap: false };
function setup() {
  const record = vi.fn().mockResolvedValue({ gap: false, duplicate: false });
  const controller = { restore: vi.fn(), get: vi.fn(() => active), record };
  const diagnostics = vi.fn();
  return { record, controller, diagnostics };
}
const message = { type: 'capture.event', sessionId: traceId, kind: 'jsf.click', eventId, metadata: { source: 'button' } };
const sender = { url: 'http://localhost:8180/page', tab: { id: 4, url: 'http://localhost:8180/page' } };

describe('background capture event boundary', () => {
  it('acknowledges a valid event only after record succeeds', async () => {
    const deps = setup();
    await expect(handleCaptureEvent(message, sender, deps.controller as never, deps.diagnostics)).resolves.toEqual({ accepted: true, gap: false, duplicate: false });
    expect(deps.record).toHaveBeenCalledWith(4, 'jsf.click', { source: 'button' }, { eventId });
  });
  it.each([
    [{ ...sender, tab: {} }, 'SENDER_TAB'],
    [{ ...sender, url: 'chrome://settings' }, 'SENDER_URL'],
    [{ ...sender, url: 'http://other.test/page' }, 'ORIGIN_MISMATCH'],
  ])('returns an explicit error for invalid sender', async (invalidSender, code) => {
    const deps = setup();
    await expect(handleCaptureEvent(message, invalidSender, deps.controller as never, deps.diagnostics)).resolves.toMatchObject({ code });
    expect(deps.record).not.toHaveBeenCalled();
  });
  it.each([
    [{ ...message, sessionId: 'c'.repeat(32) }, 'SESSION_MISMATCH'],
    [{ ...message, eventId: 'bad' }, 'EVENT_ID'],
    [{ ...message, metadata: { source: 'x', secret: 'private' } }, undefined],
  ])('validates event/session payloads before record', async (payload, code) => {
    const deps = setup();
    const result = await handleCaptureEvent(payload, sender, deps.controller as never, deps.diagnostics);
    if (code) expect(result).toMatchObject({ code });
    else expect(deps.record).toHaveBeenCalledWith(4, 'jsf.click', { source: 'x' }, { eventId });
  });
  it('rejects absent and expired sessions without recording', async () => {
    const absent = setup(); absent.controller.get.mockReturnValue(undefined as never);
    await expect(handleCaptureEvent(message, sender, absent.controller as never, absent.diagnostics)).resolves.toMatchObject({ code: 'SESSION_MISMATCH' });
    expect(absent.record).not.toHaveBeenCalled();
    const expired = setup(); expired.controller.get.mockReturnValue({ ...active,
      session: { ...active.session, expiresAt: new Date(Date.now() - 1).toISOString() } });
    await expect(handleCaptureEvent(message, sender, expired.controller as never, expired.diagnostics)).resolves.toMatchObject({ code: 'SESSION_MISMATCH' });
    expect(expired.record).not.toHaveBeenCalled();
  });
  it('returns an explicit error if record rejects', async () => {
    const deps = setup(); deps.record.mockRejectedValue(new Error('private native detail'));
    await expect(handleCaptureEvent(message, sender, deps.controller as never, deps.diagnostics)).resolves.toMatchObject({ code: 'RECORD_FAILED' });
  });
  it('ignores unrelated messages', async () => {
    const deps = setup();
    await expect(handleCaptureEvent({ type: 'other' }, sender, deps.controller as never, deps.diagnostics)).resolves.toBeUndefined();
  });
});
