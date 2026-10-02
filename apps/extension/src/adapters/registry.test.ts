import { describe, expect, it } from 'vitest';
import { PrimeFacesAdapter } from './primefaces5.ts';
import { PrimeFaces15Adapter } from './primefaces15.ts';
import { selectPrimeFacesAdapter } from './registry.ts';

describe('selectPrimeFacesAdapter', () => {
  it('keeps the PrimeFaces 5 adapter for recognized version 5', () => {
    expect(selectPrimeFacesAdapter('5.3.14')).toBeInstanceOf(PrimeFacesAdapter);
  });

  it('uses the PrimeFaces 15 adapter for version 15', () => {
    expect(selectPrimeFacesAdapter('15.0.0')).toBeInstanceOf(PrimeFaces15Adapter);
  });

  it('routes unknown versions through the experimental adapter, never the PrimeFaces 5 adapter', () => {
    const adapter = selectPrimeFacesAdapter('future-build');
    expect(adapter).toBeInstanceOf(PrimeFaces15Adapter);
    expect(adapter).not.toBeInstanceOf(PrimeFacesAdapter);
  });
});
