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
});
