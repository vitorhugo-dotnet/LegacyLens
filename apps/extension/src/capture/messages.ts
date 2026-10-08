export interface CaptureEventAcknowledgement { accepted: true; gap: boolean; duplicate?: boolean }
export type CaptureEventReply = CaptureEventAcknowledgement | { error: string; code?: string; retryable?: boolean };
