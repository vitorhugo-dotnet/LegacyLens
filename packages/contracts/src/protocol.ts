export const protocolVersion = 1 as const;

export const commands = [
  'project.register', 'project.list', 'project.status', 'project.index', 'symbol.search', 'graph.explore',
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
  includes?: string[] | null;
  excludes?: string[] | null;
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
  contentHash: string;
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
  incomplete?: boolean;
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

export interface Page<T> {
  items: T[];
  offset: number;
  limit: number;
  total: number;
  hasMore: boolean;
}

export type ProjectListResult = Page<Project>;

export interface IndexResult {
  projectId: ID;
  revisionId: ID;
  artifacts: Page<Artifact>;
  symbols: Page<Symbol>;
  relations: Page<Relation>;
  evidence: Page<Evidence>;
  diagnostics: Page<Diagnostic>;
  replacedFiles: Page<string>;
  excludedFiles: Page<string>;
}

export interface SearchResult {
  symbols: Symbol[];
  total: number;
  offset: number;
  limit: number;
  hasMore: boolean;
}

export interface GraphResult {
  symbols: Page<Symbol>;
  relations: Page<Relation>;
}

export interface ImpactQueryPayload {
  projectId: ID;
  revisionId?: ID;
  symbolId: ID;
  depth: number;
  offset: number;
  limit: number;
}

export interface ImpactQueryCommand {
  protocolVersion: 1;
  requestId: string;
  command: 'impact.query';
  payload: ImpactQueryPayload;
}

export interface ImpactPath {
  sourceId: ID;
  targetId: ID;
  symbolIds: ID[];
  relationIds: ID[];
  evidenceIds: ID[];
  inferred?: boolean;
}

export interface GraphDiagnostic {
  id: ID;
  code: string;
  message: string;
  symbolIds?: ID[];
  relationId?: ID;
  evidenceIds?: ID[];
  location?: Location;
}

export interface ImpactResult {
  projectId: ID;
  revisionId: ID;
  symbols: Symbol[];
  relations: Relation[];
  paths: ImpactPath[];
  evidence: Evidence[];
  diagnostics: GraphDiagnostic[];
  depth: number;
  offset: number;
  limit: number;
  total: number;
  hasMore: boolean;
  truncated: boolean;
}

export interface CaptureSession {
  id: ID;
  projectId: ID;
  startedAt: string;
  expiresAt: string;
}

export interface IngestResult {
  accepted: number;
  duplicate: number;
  diagnostics: Diagnostic[] | null;
}

export interface Investigation {
  project: Project;
  trace: Trace;
  agentStatus: { state: 'unknown' | 'online' | 'offline'; evidenceDiagnosticId?: ID };
  indexedRevisionId?: ID;
  events: Page<Event>;
  diagnostics: Page<Diagnostic>;
  symbols: Page<Symbol>;
  relations: Page<Relation>;
  evidence: Page<Evidence>;
}

export interface OpenResult {
  opened: boolean;
  message: string;
  file?: string;
  line?: number;
}

export interface ExplanationEvidence {
  id: ID;
  kind: string;
  path?: string;
  line?: number;
  column?: number;
}

export interface ExplanationPackage {
  question: string;
  indexedRevisionId?: ID;
  deploymentRevisions: ID[];
  traceIncomplete: boolean;
  evidence: ExplanationEvidence[];
  limitations: string[];
}

export interface ExplanationPreview {
  previewId: string;
  expiresAt: string;
  destination: string;
  providerAvailable: boolean;
  package: ExplanationPackage;
}

export interface ExplanationClaim {
  text: string;
  evidenceIds: ID[];
  confidence: 'supported' | 'hypothesis';
}

export interface ExplanationResult {
  claims: ExplanationClaim[];
  limitations: string[];
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
    && (value.toId === undefined || (typeof value.toId === 'string' && value.toId.length > 0))
    && typeof value.kind === 'string' && value.kind.length > 0
    && Array.isArray(value.evidenceIds) && value.evidenceIds.length > 0
    && value.evidenceIds.every((id) => typeof id === 'string' && id.length > 0)
    && ['resolved', 'dynamic', 'unresolved'].includes(String(value.resolution))
    && ['static', 'observed'].includes(String(value.layer))
    && (value.location === undefined || (isRecord(value.location)
      && typeof value.location.path === 'string' && value.location.path.length > 0
      && Number.isInteger(value.location.line) && Number(value.location.line) >= 1
      && Number.isInteger(value.location.column) && Number(value.location.column) >= 1));
}

function invalidPayload(message: string): never {
  throw new ProtocolValidationError('INVALID_PAYLOAD', message);
}

function nonEmptyString(value: unknown): value is string {
  return typeof value === 'string' && value.length > 0;
}

function stringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every(nonEmptyString);
}

function isEvent(value: unknown): value is Event {
  return isRecord(value) && !Object.keys(value).some((key) => !['projectId', 'traceId', 'producerId', 'sequence', 'eventId', 'parentEventId', 'kind', 'occurredAt', 'applicationRevision', 'javaDestination', 'metadata'].includes(key))
    && nonEmptyString(value.projectId)
    && typeof value.traceId === 'string' && /^[a-fA-F0-9]{32}$/.test(value.traceId)
    && nonEmptyString(value.producerId) && typeof value.sequence === 'number' && Number.isInteger(value.sequence) && value.sequence > 0
    && nonEmptyString(value.eventId) && typeof value.kind === 'string' && value.kind.trim().length > 0 && nonEmptyString(value.occurredAt) && !Number.isNaN(Date.parse(value.occurredAt))
    && (value.parentEventId === undefined || nonEmptyString(value.parentEventId))
    && (value.applicationRevision === undefined || nonEmptyString(value.applicationRevision))
    && (value.javaDestination === undefined || nonEmptyString(value.javaDestination))
    && (value.metadata === undefined || (isRecord(value.metadata)
      && Object.values(value.metadata).every((item) => typeof item === 'string')));
}

function validatePayload(command: (typeof commands)[number], payload: Record<string, unknown>): void {
  const only = (keys: string[]): void => {
    if (Object.keys(payload).some((key) => !keys.includes(key))) invalidPayload('payload contains an unsupported field');
  };
  const requireString = (key: string): void => {
    if (!nonEmptyString(payload[key])) invalidPayload(`${key} must be a non-empty string`);
  };
  const requireTraceId = (key: string): void => {
    requireString(key);
    if (!/^[a-fA-F0-9]{32}$/.test(payload[key] as string)) invalidPayload(`${key} must be a 32-character trace id`);
  };
  const optionalString = (key: string): void => {
    if (payload[key] !== undefined && typeof payload[key] !== 'string') invalidPayload(`${key} must be a string`);
  };
  const optionalStringArray = (key: string): void => {
    if (payload[key] !== undefined && !stringArray(payload[key])) invalidPayload(`${key} must be an array of non-empty strings`);
  };

  switch (command) {
    case 'project.register':
      only(['projectId', 'root', 'name', 'includes', 'excludes']);
      requireString('root');
      optionalString('name');
      optionalStringArray('includes');
      optionalStringArray('excludes');
      break;
    case 'project.list':
      only(['offset', 'limit']);
      if (payload.offset !== undefined && (typeof payload.offset !== 'number' || !Number.isInteger(payload.offset) || payload.offset < 0 || payload.offset > 1_000_000_000)) invalidPayload('offset must be an integer from 0 to 1000000000');
      if (payload.limit !== undefined && (typeof payload.limit !== 'number' || !Number.isInteger(payload.limit) || payload.limit < 0 || payload.limit > 200)) invalidPayload('limit must be an integer from 0 to 200');
      break;
    case 'project.status':
    case 'location.open':
    case 'project.index':
    case 'symbol.search':
    case 'graph.explore':
    case 'impact.query':
    case 'capture.start':
    case 'capture.stop':
    case 'trace.ingest':
    case 'investigation.get':
      only(command === 'project.status' ? ['projectId']
        : command === 'project.index' ? ['projectId', 'paths', 'offset', 'limit']
          : command === 'symbol.search' ? ['projectId', 'revisionId', 'text', 'kinds', 'limit', 'offset']
            : command === 'graph.explore' ? ['projectId', 'revisionId', 'symbolIds', 'depth', 'offset', 'limit']
              : command === 'impact.query' ? ['projectId', 'revisionId', 'symbolId', 'depth', 'offset', 'limit']
                : command === 'capture.start' ? ['projectId', 'tabId']
                    : command === 'capture.stop' ? ['projectId', 'traceId']
                    : command === 'trace.ingest' ? ['projectId', 'events']
                      : command === 'investigation.get' ? ['projectId', 'traceId', 'offset', 'limit']
                        : ['projectId', 'location']);
      requireString('projectId');
      if (command === 'project.status') break;
      if (command === 'location.open') {
        const location = payload.location;
        if (!isRecord(location) || !nonEmptyString(location.path) || !Number.isInteger(location.line) || Number(location.line) < 1
            || !Number.isInteger(location.column) || Number(location.column) < 1) invalidPayload('location must have a path and one-based line and column');
        break;
      }
      if (command === 'project.index') {
        optionalStringArray('paths');
        if (payload.offset !== undefined && (typeof payload.offset !== 'number' || !Number.isInteger(payload.offset) || payload.offset < 0 || payload.offset > 1_000_000_000)) invalidPayload('offset must be an integer from 0 to 1000000000');
        if (payload.limit !== undefined && (typeof payload.limit !== 'number' || !Number.isInteger(payload.limit) || payload.limit < 0 || payload.limit > 200)) invalidPayload('limit must be an integer from 0 to 200');
        break;
      }
      if (command === 'symbol.search') {
        requireString('text');
        optionalString('revisionId');
        optionalStringArray('kinds');
        if (payload.limit !== undefined && (typeof payload.limit !== 'number' || !Number.isInteger(payload.limit) || payload.limit < 0 || payload.limit > 200)) invalidPayload('limit must be an integer from 0 to 200');
        if (payload.offset !== undefined && (typeof payload.offset !== 'number' || !Number.isInteger(payload.offset) || payload.offset < 0 || payload.offset > 1_000_000_000)) invalidPayload('offset must be an integer from 0 to 1000000000');
        break;
      }
      if (command === 'graph.explore') {
        if (!stringArray(payload.symbolIds) || payload.symbolIds.length === 0) invalidPayload('symbolIds must contain at least one non-empty string');
        if (typeof payload.depth !== 'number' || !Number.isInteger(payload.depth) || payload.depth < 0 || payload.depth > 16) invalidPayload('depth must be an integer from 0 to 16');
        optionalString('revisionId');
        if (payload.offset !== undefined && (typeof payload.offset !== 'number' || !Number.isInteger(payload.offset) || payload.offset < 0 || payload.offset > 1_000_000_000)) invalidPayload('offset must be an integer from 0 to 1000000000');
        if (payload.limit !== undefined && (typeof payload.limit !== 'number' || !Number.isInteger(payload.limit) || payload.limit < 0 || payload.limit > 200)) invalidPayload('limit must be an integer from 0 to 200');
        break;
      }
      if (command === 'impact.query') {
        requireString('symbolId');
        optionalString('revisionId');
        if (typeof payload.depth !== 'number' || !Number.isInteger(payload.depth) || payload.depth < 0 || payload.depth > 16) invalidPayload('depth must be an integer from 0 to 16');
        if (typeof payload.offset !== 'number' || !Number.isInteger(payload.offset) || payload.offset < 0 || payload.offset > 1_000_000_000) invalidPayload('offset must be an integer from 0 to 1000000000');
        if (typeof payload.limit !== 'number' || !Number.isInteger(payload.limit) || payload.limit < 0 || payload.limit > 200) invalidPayload('limit must be an integer from 0 to 200');
        break;
      }
      if (command === 'capture.start') {
        optionalString('tabId');
        break;
      }
      if (command === 'capture.stop') {
        requireTraceId('traceId');
      } else if (command === 'trace.ingest') {
        if (!Array.isArray(payload.events) || payload.events.length > 10_000
            || !payload.events.every((event) => isEvent(event) && event.projectId === payload.projectId)) invalidPayload('events must contain valid events for the requested project');
      } else {
        if (command === 'investigation.get') {
          requireTraceId('traceId');
          if (payload.offset !== undefined && (typeof payload.offset !== 'number' || !Number.isInteger(payload.offset) || payload.offset < 0 || payload.offset > 1_000_000_000)) invalidPayload('offset must be an integer from 0 to 1000000000');
          if (payload.limit !== undefined && (typeof payload.limit !== 'number' || !Number.isInteger(payload.limit) || payload.limit < 0 || payload.limit > 200)) invalidPayload('limit must be an integer from 0 to 200');
        }
      }
      break;
    case 'explanation.preview':
      only(['projectId', 'traceId', 'question', 'evidenceIds']);
      requireString('projectId');
      requireTraceId('traceId');
      requireString('question');
      if ((payload.question as string).trim().length === 0 || (payload.question as string).length > 2000) invalidPayload('question must contain 1 to 2000 characters');
      if (!Array.isArray(payload.evidenceIds) || payload.evidenceIds.length < 1 || payload.evidenceIds.length > 20
          || !stringArray(payload.evidenceIds) || new Set(payload.evidenceIds).size !== payload.evidenceIds.length) invalidPayload('evidenceIds must contain 1 to 20 unique evidence ids');
      break;
    case 'explanation.generate':
      only(['previewId', 'consent']);
      requireString('previewId');
      if (payload.consent !== true) invalidPayload('explanation generation requires explicit consent');
      break;
  }
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
  if (typeof input.command !== 'string' || input.command.length === 0) {
    throw new ProtocolValidationError('INVALID_ENVELOPE', 'Envelope command must be a non-empty string');
  }
  if (!commands.includes(input.command as (typeof commands)[number])) {
    throw new ProtocolValidationError('UNSUPPORTED_COMMAND', 'Envelope command is not supported');
  }
  if (!isRecord(input.payload)) {
    throw new ProtocolValidationError('INVALID_ENVELOPE', 'Envelope payload must be a JSON object');
  }
  if (input.payload.relations !== undefined) {
    if (!Array.isArray(input.payload.relations) || !input.payload.relations.every(isRelation)) {
      throw new ProtocolValidationError('INVALID_RELATION', 'Each relation must have valid fields and at least one evidence ID');
    }
  }
  validatePayload(input.command as (typeof commands)[number], input.payload);
  return input as unknown as Envelope;
}
