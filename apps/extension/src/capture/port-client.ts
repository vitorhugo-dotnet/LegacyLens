import { CAPTURE_PORT_NAME, CAPTURE_PORT_RECONNECT_DELAYS_MS, CAPTURE_PORT_VERSION, MAX_PENDING_CAPTURE_EVENTS,
  parseCapturePortServerMessage, type CapturePortClientMessage, type CapturePortServerMessage } from './port-protocol.ts';

export interface CapturePortEvent {
  eventId: string; kind: string; metadata: Record<string, string>; parentEventId?: string;
}
export interface CapturePortAcknowledgement { accepted: true; gap: boolean; duplicate: boolean }
export interface CapturePortLike {
  postMessage(message: unknown): void;
  disconnect(): void;
  onMessage: { addListener(listener: (message: unknown) => void): void; removeListener(listener: (message: unknown) => void): void };
  onDisconnect: { addListener(listener: () => void): void; removeListener(listener: () => void): void };
}
interface Pending { event: CapturePortEvent; resolve(value: CapturePortAcknowledgement): void; reject(error: Error): void; settled: boolean }
function randomEventId(): string {
  let id = '';
  do { id = [...crypto.getRandomValues(new Uint8Array(8))].map((n) => n.toString(16).padStart(2, '0')).join(''); } while (/^0+$/.test(id));
  return id;
}

export class CapturePortClient {
  private traceId?: string;
  private port: CapturePortLike | undefined;
  private queue: Pending[] = [];
  private inFlight: Pending | undefined;
  private ready = false;
  private connecting = false;
  private stopped = false;
  private closed = false;
  private hasGap = false;
  private retries = 0;
  private retryTimer: ReturnType<typeof setTimeout> | undefined;
  private ackTimer: ReturnType<typeof setTimeout> | undefined;
  private startPromise: Promise<void> | undefined;
  private startResolve: (() => void) | undefined;
  private startReject: ((error: Error) => void) | undefined;
  private readonly drainWaiters = new Set<() => void>();

  constructor(private readonly connect: (name: string) => CapturePortLike,
    private readonly diagnostics: (stage: string, eventId?: string, code?: string) => void = () => {},
    private readonly timers = { setTimeout, clearTimeout }) {}

  start(traceId: string): Promise<void> {
    if (this.startPromise) return this.startPromise;
    this.traceId = traceId;
    this.stopped = false;
    this.startPromise = new Promise<void>((resolve, reject) => { this.startResolve = resolve; this.startReject = reject; });
    this.open();
    return this.startPromise;
  }

  send(event: CapturePortEvent): Promise<CapturePortAcknowledgement> {
    if (this.stopped || !this.traceId) return Promise.reject(new Error('Capture Port is not active'));
    if (this.queue.length >= MAX_PENDING_CAPTURE_EVENTS) {
      this.hasGap = true;
      const eventId = randomEventId();
      if (this.ready && this.port) this.safePost({ type: 'overflow', version: CAPTURE_PORT_VERSION, traceId: this.traceId, eventId });
      this.diagnostics('queue.overflow', event.eventId, 'QUEUE_FULL');
      return Promise.reject(new Error('Capture Port queue is full'));
    }
    const pending = {} as Pending;
    pending.event = event;
    pending.settled = false;
    const result = new Promise<CapturePortAcknowledgement>((resolve, reject) => { pending.resolve = resolve; pending.reject = reject; });
    this.queue.push(pending);
    this.pump();
    return result;
  }

  async stop(_code = 'CAPTURE_STOPPED'): Promise<boolean> {
    if (this.stopped) return this.hasGap;
    this.stopped = true;
    const deadline = Date.now() + 35_000;
    while (this.queue.length && Date.now() < deadline) {
      await new Promise<void>((resolve) => {
        let settled = false;
        const finish = () => { if (settled) return; settled = true; this.drainWaiters.delete(finish); this.timers.clearTimeout(timer); resolve(); };
        const timer = this.timers.setTimeout(finish, Math.max(0, deadline - Date.now()));
        this.drainWaiters.add(finish);
      });
    }
    if (this.queue.length) this.hasGap = true;
    if (this.retryTimer) this.timers.clearTimeout(this.retryTimer);
    this.retryTimer = undefined;
    this.closed = true;
    this.failAll(new Error('Capture Port stopped before all events were acknowledged'));
    this.disconnect();
    return this.hasGap;
  }

  private open(): void {
    if (this.closed || this.connecting) return;
    this.connecting = true;
    try {
      const port = this.connect(CAPTURE_PORT_NAME);
      this.port = port;
      port.onMessage.addListener(this.onMessage);
      port.onDisconnect.addListener(this.onDisconnect);
      if (!this.traceId) throw new Error('Capture trace is missing');
      this.safePost({ type: 'hello', version: CAPTURE_PORT_VERSION, traceId: this.traceId });
      this.diagnostics('port.connect');
      this.ackTimer = this.timers.setTimeout(() => { this.diagnostics('port.handshake-timeout', undefined, 'TIMEOUT'); this.reconnect(); }, 35_000);
    } catch {
      this.connecting = false;
      this.port = undefined;
      this.scheduleReconnect();
    }
  }

  private onMessage = (value: unknown): void => {
    const message = parseCapturePortServerMessage(value);
    if (!message) { this.diagnostics('protocol.invalid', undefined, 'INVALID_MESSAGE'); this.reconnect(); return; }
    if (message.traceId !== this.traceId) return;
    if (message.type === 'ready') {
      this.clearAckTimer();
      this.ready = true;
      this.connecting = false;
      this.startResolve?.();
      this.startResolve = undefined;
      this.startReject = undefined;
      this.diagnostics('port.ready');
      this.pump();
      return;
    }
    const current = this.inFlight;
    if (message.type === 'ack' && current && message.eventId === current.event.eventId) {
      this.clearAckTimer();
      this.inFlight = undefined;
      this.queue.shift();
      current.settled = true;
      current.resolve({ accepted: true, gap: message.gap, duplicate: message.duplicate });
      this.notifyDrain();
      this.diagnostics('event.ack', message.eventId);
      this.pump();
    } else if (message.type === 'nack' && message.retryable && (!message.eventId || this.inFlight?.event.eventId === message.eventId)) {
      this.diagnostics('event.retryable-nack', message.eventId, message.code);
      this.reconnect();
    } else if (message.type === 'nack' && (!message.eventId || current?.event.eventId === message.eventId)) {
      if (!current) { this.failAll(new Error(message.code)); return; }
      this.hasGap = true;
      this.diagnostics('event.nack', message.eventId, message.code);
      this.clearAckTimer();
      if (current) { current.settled = true; current.reject(new Error(message.code)); }
      this.inFlight = undefined;
      if (current) this.queue.shift();
      this.notifyDrain();
      this.pump();
    }
  };

  private onDisconnect = (): void => {
    this.diagnostics('port.disconnect', this.inFlight?.event.eventId, 'PORT_CLOSED');
    this.disconnect();
    this.scheduleReconnect();
  };

  private reconnect(): void {
    this.disconnect();
    this.scheduleReconnect();
  }

  private scheduleReconnect(): void {
    if (this.closed || this.retryTimer) return;
    const delay = CAPTURE_PORT_RECONNECT_DELAYS_MS[this.retries];
    if (delay === undefined) {
      this.hasGap = this.queue.length > 0 || this.hasGap;
      this.diagnostics('port.reconnect-exhausted', this.inFlight?.event.eventId, 'RECONNECT_EXHAUSTED');
      this.failAll(new Error('Capture Port reconnect limit exhausted'));
      return;
    }
    this.retries++;
    this.diagnostics('port.reconnect');
    this.retryTimer = this.timers.setTimeout(() => {
      this.retryTimer = undefined;
      this.open();
    }, delay);
  }

  private pump(): void {
    if (!this.ready || this.inFlight || !this.queue.length || !this.traceId) return;
    const current = this.queue[0]!;
    this.inFlight = current;
    const message: CapturePortClientMessage = { type: 'event', version: CAPTURE_PORT_VERSION, traceId: this.traceId,
      eventId: current.event.eventId, kind: current.event.kind, ...(current.event.parentEventId ? { parentEventId: current.event.parentEventId } : {}),
      metadata: current.event.metadata };
    if (!this.safePost(message)) return;
    this.diagnostics('event.send', current.event.eventId);
    this.ackTimer = this.timers.setTimeout(() => {
      this.diagnostics('event.timeout', current.event.eventId, 'TIMEOUT');
      this.reconnect();
    }, 35_000);
  }

  private safePost(message: CapturePortClientMessage): boolean {
    try { this.port?.postMessage(message); return Boolean(this.port); }
    catch { this.disconnect(); this.scheduleReconnect(); return false; }
  }

  private disconnect(): void {
    this.ready = false;
    this.connecting = false;
    this.clearAckTimer();
    this.inFlight = undefined;
    const old = this.port;
    this.port = undefined;
    if (old) {
      old.onMessage.removeListener(this.onMessage);
      old.onDisconnect.removeListener(this.onDisconnect);
      try { old.disconnect(); } catch { /* already disconnected */ }
    }
  }

  private clearAckTimer(): void { if (this.ackTimer) this.timers.clearTimeout(this.ackTimer); this.ackTimer = undefined; }
  private notifyDrain(): void { for (const waiter of [...this.drainWaiters]) waiter(); }
  private failAll(error: Error): void {
    this.clearAckTimer();
    if (this.startReject) { this.startReject(error); this.startReject = undefined; this.startResolve = undefined; }
    for (const item of this.queue.splice(0)) if (!item.settled) { item.settled = true; item.reject(error); }
    this.inFlight = undefined;
    this.notifyDrain();
  }
}
