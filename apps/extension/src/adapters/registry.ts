import { PrimeFacesAdapter } from './primefaces5.ts';
import { PrimeFaces15Adapter } from './primefaces15.ts';

export type PagePrimeFacesAdapter = PrimeFacesAdapter | PrimeFaces15Adapter;

type PrimeFacesRuntimeIdentity = { VERSION?: unknown; ajax?: { Request?: { handle?: unknown } } };

/** Select by explicit version or the distinctive PF5 request hook when legacy builds omit VERSION. */
export function selectPrimeFacesAdapter(runtime: unknown): PagePrimeFacesAdapter {
  const identity = runtime && typeof runtime === 'object' ? runtime as PrimeFacesRuntimeIdentity : undefined;
  const version = identity ? identity.VERSION : runtime;
  const isKnown5 = typeof version === 'string' && /^5(?:\.|$)/.test(version);
  const hasLegacyRequestHook = version == null && typeof identity?.ajax?.Request?.handle === 'function';
  if (isKnown5 || hasLegacyRequestHook) return new PrimeFacesAdapter();
  return new PrimeFaces15Adapter();
}
