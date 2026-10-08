import { commands, protocolVersion, type ResponseEnvelope } from '@legacylens/contracts/src/protocol.ts';

export type Command = (typeof commands)[number];
export interface CommandClient { request<T>(command: Command, payload: unknown): Promise<T> }

export type NativeRequestErrorCode = 'timeout' | 'disconnected' | 'post-failed' | 'host-rejected';
export class NativeRequestError extends Error {
  constructor(readonly code: NativeRequestErrorCode, message: string) {
    super(message);
    this.name = 'NativeRequestError';
  }
}

export class NativeClient implements CommandClient {
  private port: chrome.runtime.Port | undefined;
  private pending = new Map<string, { resolve(value: unknown): void; reject(error: Error): void; timer: ReturnType<typeof setTimeout> }>();

  constructor(private readonly host = 'io.legacylens.host') {}

  async request<T>(command: Command, payload: unknown): Promise<T> {
    if (!payload || typeof payload !== 'object' || Array.isArray(payload)) throw new Error('Native payload must be an object');
    const requestId = crypto.randomUUID();
    const envelope = { protocolVersion, requestId, command, payload };
    if (new TextEncoder().encode(JSON.stringify(envelope)).length > 512 * 1024) throw new Error('Native request exceeds 512 KiB');
    if (!this.port) this.connect();
    return new Promise<T>((resolve, reject) => {
      const timer = setTimeout(() => { this.pending.delete(requestId); reject(new NativeRequestError('timeout', 'Native host request timed out')); }, 30_000);
      this.pending.set(requestId, { resolve: (value) => resolve(value as T), reject, timer });
      try { this.port!.postMessage(envelope); }
      catch { clearTimeout(timer); this.pending.delete(requestId); reject(new NativeRequestError('post-failed', 'Native host unavailable')); }
    });
  }

  private connect(): void {
    const port = chrome.runtime.connectNative(this.host);
    this.port = port;
    port.onMessage.addListener((message: ResponseEnvelope) => {
      if (message.protocolVersion !== protocolVersion || typeof message.requestId !== 'string') return;
      const pending = this.pending.get(message.requestId);
      if (!pending) return;
      this.pending.delete(message.requestId);
      clearTimeout(pending.timer);
      if (message.error) pending.reject(new NativeRequestError('host-rejected', `${message.error.code}: ${message.error.message}`));
      else pending.resolve(message.result);
    });
    port.onDisconnect.addListener(() => {
      this.port = undefined;
      for (const pending of this.pending.values()) { clearTimeout(pending.timer); pending.reject(new NativeRequestError('disconnected', 'Native host disconnected')); }
      this.pending.clear();
    });
  }
}
