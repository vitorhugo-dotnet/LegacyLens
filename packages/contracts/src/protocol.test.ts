import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { parseEnvelope, ProtocolValidationError } from './protocol.ts';

const fixture = (name: string): unknown => JSON.parse(readFileSync(new URL(`../../../contracts/examples/${name}.json`, import.meta.url), 'utf8'));

describe('parseEnvelope', () => {
  it('accepts the shared valid interaction example as protocol version 1', () => {
    expect(parseEnvelope(fixture('valid-interaction')).protocolVersion).toBe(1);
  });

  it('rejects a different protocol version with a typed error', () => {
    const invalidVersion = fixture('invalid-version');
    expect(() => parseEnvelope(invalidVersion)).toThrowError(ProtocolValidationError);
    try {
      parseEnvelope(invalidVersion);
    } catch (error) {
      expect(error).toMatchObject({ code: 'UNSUPPORTED_PROTOCOL_VERSION' });
    }
  });

  it('rejects a relation without evidence with a typed error', () => {
    const invalidRelation = fixture('invalid-relation');
    expect(() => parseEnvelope(invalidRelation)).toThrowError(ProtocolValidationError);
    try {
      parseEnvelope(invalidRelation);
    } catch (error) {
      expect(error).toMatchObject({ code: 'INVALID_RELATION' });
    }
  });

  it('rejects an empty relation target and empty location path', () => {
    const envelope = fixture('invalid-relation') as { payload: { relations: Array<Record<string, unknown>> } };
    const relation = envelope.payload.relations[0]!;
    relation.evidenceIds = ['evidence-1'];
    relation.toId = '';
    expect(() => parseEnvelope(envelope)).toThrowError(ProtocolValidationError);
    expect(() => parseEnvelope(envelope)).toThrowError(expect.objectContaining({ code: 'INVALID_RELATION' }));

    const location = fixture('invalid-relation') as { payload: { relations: Array<{ evidenceIds: string[]; location: Record<string, unknown> }> } };
    const locatedRelation = location.payload.relations[0]!;
    locatedRelation.evidenceIds = ['evidence-1'];
    locatedRelation.location = { path: 'view.xhtml', line: 1, column: 1 };
    locatedRelation.location.path = '';
    expect(() => parseEnvelope(location)).toThrowError(expect.objectContaining({ code: 'INVALID_RELATION' }));
  });

  it('reports a typed unsupported error for a future command', () => {
    expect(() => parseEnvelope({ protocolVersion: 1, requestId: 'r1', command: 'project.archive', payload: {} }))
      .toThrowError(expect.objectContaining({ code: 'UNSUPPORTED_COMMAND' }));
  });

  it('validates the concrete project registration payload', () => {
    expect(() => parseEnvelope({ protocolVersion: 1, requestId: 'r1', command: 'project.register', payload: { root: 42 } }))
      .toThrowError(expect.objectContaining({ code: 'INVALID_PAYLOAD' }));
    expect(parseEnvelope({ protocolVersion: 1, requestId: 'r1', command: 'project.register', payload: { root: 'C:/source', includes: ['**/*.java'], excludes: [] } }).command)
      .toBe('project.register');
  });

  it('accepts a valid location.open payload without a trace id', () => {
    expect(parseEnvelope({ protocolVersion: 1, requestId: 'r1', command: 'location.open', payload: {
      projectId: 'project-1', location: { path: 'views/home.xhtml', line: 4, column: 2 },
    } }).command).toBe('location.open');
  });

  it('rejects ingested events without a positive sequence or non-empty kind', () => {
    const event = { projectId: 'p1', traceId: '0123456789abcdef0123456789abcdef', producerId: 'agent', sequence: 1,
      eventId: 'event-1', kind: 'http.request', occurredAt: '2026-09-30T12:00:00Z' };
    for (const invalid of [{ ...event, sequence: 0 }, { ...event, kind: '' }]) {
      expect(() => parseEnvelope({ protocolVersion: 1, requestId: 'r1', command: 'trace.ingest',
        payload: { projectId: 'p1', events: [invalid] } })).toThrowError(expect.objectContaining({ code: 'INVALID_PAYLOAD' }));
    }
  });

  it('validates bounded offset and limit fields on paged commands', () => {
    expect(parseEnvelope({ protocolVersion: 1, requestId: 'r1', command: 'project.list', payload: { offset: 200, limit: 200 } }).command)
      .toBe('project.list');
    expect(() => parseEnvelope({ protocolVersion: 1, requestId: 'r1', command: 'project.list', payload: { offset: 0, limit: 201 } }))
      .toThrowError(expect.objectContaining({ code: 'INVALID_PAYLOAD' }));
    expect(parseEnvelope({ protocolVersion: 1, requestId: 'r1', command: 'symbol.search', payload: { projectId: 'p1', text: 'save', offset: 400, limit: 20 } }).command)
      .toBe('symbol.search');
    expect(parseEnvelope({ protocolVersion: 1, requestId: 'r1', command: 'investigation.get', payload: { projectId: 'p1', traceId: '0123456789abcdef0123456789abcdef', offset: 20, limit: 20 } }).command)
      .toBe('investigation.get');
  });

  it('requires bounded depth and explicit pagination on impact queries', () => {
    const payload = { projectId: 'p1', symbolId: 'orders-table', depth: 8, offset: 0, limit: 50 };
    expect(parseEnvelope({ protocolVersion: 1, requestId: 'r1', command: 'impact.query', payload }).command)
      .toBe('impact.query');
    for (const invalid of [
      { ...payload, depth: undefined },
      { ...payload, depth: 17 },
      { ...payload, offset: undefined },
      { ...payload, limit: undefined },
      { ...payload, limit: 201 },
    ]) {
      expect(() => parseEnvelope({ protocolVersion: 1, requestId: 'r1', command: 'impact.query', payload: invalid }))
        .toThrowError(expect.objectContaining({ code: 'INVALID_PAYLOAD' }));
    }
  });
});
