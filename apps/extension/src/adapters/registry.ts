import { PrimeFacesAdapter } from './primefaces5.ts';
import { PrimeFaces15Adapter } from './primefaces15.ts';

export type PagePrimeFacesAdapter = PrimeFacesAdapter | PrimeFaces15Adapter;

/** Select by an explicit major version. Unknown versions remain experimental and never fall back to PF5. */
export function selectPrimeFacesAdapter(version: unknown): PagePrimeFacesAdapter {
  if (typeof version === 'string' && /^5(?:\.|$)/.test(version)) return new PrimeFacesAdapter();
  return new PrimeFaces15Adapter();
}
