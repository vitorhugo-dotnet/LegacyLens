export const CAPTURE_TRANSPORT_STORAGE_PREFIX = 'legacylens.capture.requests.v1.';

export interface CaptureStorageRequest {
  requestId: string;
  tabId: number;
  origin: string;
  message: Record<string, unknown>;
}

export function captureStorageKey(requestId: string): string {
  return `${CAPTURE_TRANSPORT_STORAGE_PREFIX}${requestId}`;
}

export function parseCaptureStorageRequest(key: string, value: unknown): CaptureStorageRequest | undefined {
  if (!key.startsWith(CAPTURE_TRANSPORT_STORAGE_PREFIX)) return;
  const requestId = key.slice(CAPTURE_TRANSPORT_STORAGE_PREFIX.length);
  if (!/^(?!0{16})[a-f0-9]{16}$/i.test(requestId) || !value || typeof value !== 'object' || Array.isArray(value)) return;
  const request = value as Record<string, unknown>;
  if (request.requestId !== requestId || !Number.isSafeInteger(request.tabId) || (request.tabId as number) < 0
    || typeof request.origin !== 'string' || request.origin.length > 512
    || !request.message || typeof request.message !== 'object' || Array.isArray(request.message)) return;
  return request as unknown as CaptureStorageRequest;
}
