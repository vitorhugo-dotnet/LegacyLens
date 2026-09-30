export const protocolVersion = 1 as const;

export const commands = [
  'project.register', 'project.index', 'symbol.search', 'graph.explore',
  'impact.query', 'capture.start', 'capture.stop', 'trace.ingest',
  'investigation.get', 'location.open', 'explanation.preview', 'explanation.generate',
] as const;

export type ID = string;

export interface Location {
  path: string;
  line: number;
  column: number;
}

export interface Project {
  id: ID;
  name: string;
  root: string;
  createdAt: string;
}

export interface Revision {
  id: ID;
  projectId: ID;
  digest: string;
  createdAt: string;
}

export interface Artifact {
  id: ID;
  projectId: ID;
  revisionId: ID;
  path: string;
  language: string;
  origin: string;
}

export interface Symbol {
  id: ID;
  projectId: ID;
  revisionId: ID;
  artifactId: ID;
  path: string;
  qualifiedName: string;
  descriptor: string;
  kind: string;
  location?: Location;
}

export type Resolution = 'resolved' | 'dynamic' | 'unresolved';
export type Layer = 'static' | 'observed';

export interface Relation {
  id: ID;
  fromId: ID;
  toId?: ID;
  kind: string;
  evidenceIds: ID[];
  resolution: Resolution;
  layer: Layer;
  location?: Location;
}

export interface Evidence {
  id: ID;
  kind: string;
  source: string;
  location?: Location;
  observedAt?: string;
}

export interface Interaction {
  id: ID;
  projectId: ID;
  traceId: ID;
  occurredAt: string;
  eventIds: ID[];
}

export interface Trace {
  id: ID;
  projectId: ID;
  startedAt: string;
  endedAt?: string;
}

export interface Event {
  projectId: ID;
  traceId: string;
  producerId: ID;
  sequence: number;
  eventId: ID;
  parentEventId?: ID;
  kind: string;
  occurredAt: string;
  applicationRevision?: ID;
  javaDestination?: ID;
  metadata?: Record<string, string>;
}

export interface Diagnostic {
  id: ID;
  code: string;
  message: string;
  severity: string;
  location?: Location;
  createdAt: string;
}

export interface ProtocolErrorPayload {
  code: string;
  message: string;
  diagnosticIds: string[];
}

export interface Envelope {
  protocolVersion: 1;
  requestId: string;
  command: (typeof commands)[number];
  payload: Record<string, unknown>;
}

export interface ResponseEnvelope {
  protocolVersion: 1;
  requestId: string;
  result?: unknown;
  error?: ProtocolErrorPayload;
}

export class ProtocolValidationError extends Error {
  readonly code: string;

  constructor(code: string, message: string) {
    super(message);
    this.name = 'ProtocolValidationError';
    this.code = code;
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isRelation(value: unknown): boolean {
  if (!isRecord(value)) return false;
  return typeof value.id === 'string' && value.id.length > 0
    && typeof value.fromId === 'string' && value.fromId.length > 0
    && (value.toId === undefined || typeof value.toId === 'string')
    && typeof value.kind === 'string' && value.kind.length > 0
    && Array.isArray(value.evidenceIds) && value.evidenceIds.length > 0
    && value.evidenceIds.every((id) => typeof id === 'string' && id.length > 0)
    && ['resolved', 'dynamic', 'unresolved'].includes(String(value.resolution))
    && ['static', 'observed'].includes(String(value.layer))
    && (value.location === undefined || (isRecord(value.location)
      && typeof value.location.path === 'string'
      && Number.isInteger(value.location.line) && Number(value.location.line) >= 1
      && Number.isInteger(value.location.column) && Number(value.location.column) >= 1));
}

export function parseEnvelope(input: unknown): Envelope {
  if (!isRecord(input)) {
    throw new ProtocolValidationError('INVALID_ENVELOPE', 'Envelope must be a JSON object');
  }
  if (input.protocolVersion !== protocolVersion) {
    throw new ProtocolValidationError('UNSUPPORTED_PROTOCOL_VERSION', `Unsupported protocol version: ${String(input.protocolVersion)}`);
  }
  if (typeof input.requestId !== 'string' || input.requestId.length === 0) {
    throw new ProtocolValidationError('INVALID_ENVELOPE', 'Envelope requestId must be a non-empty string');
  }
  if (typeof input.command !== 'string' || !commands.includes(input.command as (typeof commands)[number])) {
    throw new ProtocolValidationError('INVALID_ENVELOPE', 'Envelope command is not supported');
  }
  if (!isRecord(input.payload)) {
    throw new ProtocolValidationError('INVALID_ENVELOPE', 'Envelope payload must be a JSON object');
  }
  if (input.payload.relations !== undefined) {
    if (!Array.isArray(input.payload.relations) || !input.payload.relations.every(isRelation)) {
      throw new ProtocolValidationError('INVALID_RELATION', 'Each relation must have valid fields and at least one evidence ID');
    }
  }
  return input as unknown as Envelope;
}
