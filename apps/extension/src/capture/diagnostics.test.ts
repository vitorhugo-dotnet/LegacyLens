import { describe, expect, it, vi } from 'vitest';
import type { CaptureStageRecord } from './diagnostics.ts';

const diagnosticsModule = await import('./diagnostics.ts').catch(() => ({} as typeof import('./diagnostics.ts')));

describe('capture stage diagnostics', () => {
  it('emits only allowlisted fields in fixture builds', () => {
    expect(typeof diagnosticsModule.recordCaptureStage).toBe('function');
    const sink = vi.fn();

    diagnosticsModule.recordCaptureStage({
      stage: 'content.send', outcome: 'started', traceId: 'a'.repeat(32), tabId: 12,
      eventId: 'b'.repeat(16), code: 'BACKGROUND_REJECTED', metadata: { source: 'secret-form-id' }, error: 'password=secret',
    } as CaptureStageRecord, { host_permissions: ['http://127.0.0.1/*'] }, sink);

    expect(sink).toHaveBeenCalledWith({ stage: 'content.send', outcome: 'started', traceId: 'a'.repeat(32),
      tabId: 12, eventId: 'b'.repeat(16), code: 'BACKGROUND_REJECTED' });
    expect(JSON.stringify(sink.mock.calls)).not.toContain('password=');
  });

  it('does not emit diagnostics in production builds', () => {
    expect(typeof diagnosticsModule.recordCaptureStage).toBe('function');
    const sink = vi.fn();

    diagnosticsModule.recordCaptureStage({ stage: 'content.send', outcome: 'started', traceId: 'a'.repeat(32), tabId: 12 },
      { host_permissions: ['https://app.example/*'] }, sink);

    expect(sink).not.toHaveBeenCalled();
  });
});
