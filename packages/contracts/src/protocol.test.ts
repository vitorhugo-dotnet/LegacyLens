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
});
