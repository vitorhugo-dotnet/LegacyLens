import type { CaptureSession, Event } from '@legacylens/contracts/src/protocol.ts';
import type { CommandClient } from '../native/client.ts';
import { NativeRequestError } from '../native/client.ts';
import type { CaptureStageRecord } from './diagnostics.ts';

export interface CaptureRequest { projectId: string; tabId: number; origin: string }
export interface ActiveCapture { request: CaptureRequest; session: CaptureSession; producerId: string; sequence: number; gap: boolean }
export interface SessionStore { load(): Promise<ActiveCapture[]>; save(captures: ActiveCapture[]): Promise<void> }
export interface EventIdentity { eventId?: string; parentEventId?: string }
export type CaptureControllerDiagnostics = (record: CaptureStageRecord) => void;

function randomHex(bytes: number): string {
  return [...crypto.getRandomValues(new Uint8Array(bytes))].map((byte) => byte.toString(16).padStart(2, '0')).join('');
}

export class CaptureController {
  private captures = new Map<number, ActiveCapture>();
  private starting = new Set<number>();
  private stopping = new Set<number>();
  private recording = new Map<number, Promise<void>>();
  private loading?: Promise<void>;

  constructor(private readonly client: CommandClient, private readonly store?: SessionStore,
    private readonly diagnostics: CaptureControllerDiagnostics = () => {}) {}

  async restore(): Promise<void> {
    if (!this.store) return;
    this.loading ??= this.store.load().then((entries) => {
      for (const entry of entries) {
        if (Date.parse(entry.session.expiresAt) > Date.now() && /^[a-f0-9]{32}$/i.test(entry.session.id)) {
          entry.gap = true;
          this.captures.set(entry.request.tabId, entry);
        }
      }
    });
    await this.loading;
  }

  get(tabId: number): ActiveCapture | undefined { return this.captures.get(tabId); }

  async start(request: CaptureRequest): Promise<CaptureSession> {
    await this.restore();
    if (this.captures.has(request.tabId) || this.starting.has(request.tabId)) throw new Error('Capture already active in this tab');
    if (!/^https?:\/\/[^/]+$/.test(request.origin)) throw new Error('Unsupported origin');
    this.starting.add(request.tabId);
    try {
      const session = await this.client.request<CaptureSession>('capture.start', { projectId: request.projectId, tabId: String(request.tabId) });
      if (!/^[a-f0-9]{32}$/i.test(session.id) || session.projectId !== request.projectId) throw new Error('Invalid capture session');
      const entry: ActiveCapture = { request, session, producerId: randomHex(8), sequence: 0, gap: false };
      this.captures.set(request.tabId, entry);
      try { await this.persist(); }
      catch (storageError) {
        let rollbackError: unknown;
        try { await this.client.request('capture.stop', { projectId: request.projectId, traceId: session.id }); }
        catch (error) { rollbackError = error; }
        if (rollbackError) {
          throw new Error('Capture state could not be saved and native rollback failed; capture remains active', { cause: rollbackError });
        }
        this.captures.delete(request.tabId);
        throw new Error('Capture state could not be saved; native capture was stopped', { cause: storageError });
      }
      return session;
    } finally { this.starting.delete(request.tabId); }
  }

  async stop(tabId: number): Promise<void> {
    if (this.stopping.has(tabId)) throw new Error('Capture is already stopping');
    this.stopping.add(tabId);
    try {
      await this.restore();
      const pending = this.recording.get(tabId);
      if (pending) await pending;
      const entry = this.captures.get(tabId);
      if (!entry) return;
      if (entry.gap) {
        await this.sendEvent(entry, 'extension.gap', { code: 'CAPTURE_RECONNECTED' });
        entry.gap = false;
        await this.persist();
      }
      await this.client.request('capture.stop', { projectId: entry.request.projectId, traceId: entry.session.id });
      this.captures.delete(tabId);
      await this.persist();
    } finally { this.stopping.delete(tabId); }
  }

  async record(tabId: number, kind: string, metadata: Record<string, string>, identity: EventIdentity = {}): Promise<void> {
    if (this.stopping.has(tabId)) throw new Error('Capture is stopping');
    for (const id of [identity.eventId, identity.parentEventId]) {
      if (id !== undefined && (!/^[a-f0-9]{16}$/i.test(id) || /^0+$/.test(id))) throw new Error('Invalid capture event identity');
    }
    const previous = this.recording.get(tabId) ?? Promise.resolve();
    const next = previous.catch(() => {}).then(() => this.recordNow(tabId, kind, metadata, identity));
    this.recording.set(tabId, next);
    try { await next; } finally { if (this.recording.get(tabId) === next) this.recording.delete(tabId); }
  }

  private async recordNow(tabId: number, kind: string, metadata: Record<string, string>, identity: EventIdentity): Promise<void> {
    await this.restore();
    const entry = this.captures.get(tabId);
    if (!entry || Date.parse(entry.session.expiresAt) <= Date.now()) return;
    try {
      if (entry.gap) {
        await this.sendEvent(entry, 'extension.gap', { code: 'CAPTURE_RECONNECTED' });
        entry.gap = false;
        await this.persist();
      }
      await this.sendEvent(entry, kind, metadata, identity);
    } catch {
      entry.gap = true;
      await this.persist();
      throw new Error('Capture transport interrupted; investigation has a gap');
    }
  }

  private async sendEvent(entry: ActiveCapture, kind: string, metadata: Record<string, string>, identity: EventIdentity = {}): Promise<void> {
    const event: Event = { projectId: entry.request.projectId, traceId: entry.session.id, producerId: entry.producerId,
      sequence: ++entry.sequence, eventId: identity.eventId ?? randomHex(8), ...(identity.parentEventId ? { parentEventId: identity.parentEventId } : {}),
      kind, occurredAt: new Date().toISOString(), metadata };
    await this.persist();
    const correlation = { traceId: entry.session.id, tabId: entry.request.tabId, eventId: event.eventId };
    this.diagnostics({ stage: 'host.ingest', outcome: 'started', ...correlation });
    try {
      await this.client.request('trace.ingest', { projectId: entry.request.projectId, events: [event] });
      this.diagnostics({ stage: 'host.ingest', outcome: 'accepted', ...correlation });
    } catch (error) {
      const code = error instanceof NativeRequestError ? error.code.toUpperCase().replaceAll('-', '_') : 'NATIVE_ERROR';
      this.diagnostics({ stage: 'host.ingest', outcome: error instanceof NativeRequestError && error.code === 'timeout' ? 'timeout' : 'rejected', ...correlation, code });
      throw error;
    }
  }

  private async persist(): Promise<void> { await this.store?.save([...this.captures.values()]); }
}
