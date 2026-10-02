import { PrimeFacesAdapter } from './primefaces5.ts';
import { PrimeFaces15Adapter } from './primefaces15.ts';

export type PagePrimeFacesAdapter = PrimeFacesAdapter | PrimeFaces15Adapter;

type PrimeFacesRuntimeIdentity = { VERSION?: unknown; ajax?: { Request?: { handle?: unknown } } };
type PageRuntimeIdentity = { PrimeFaces?: PrimeFacesRuntimeIdentity; jQuery?: { ajaxPrefilter?: unknown } };

/** Select by explicit version or the distinctive PF5 request hook when legacy builds omit VERSION. */
export function selectPrimeFacesAdapter(runtime: unknown): PagePrimeFacesAdapter {
  const page = runtime && typeof runtime === 'object' && 'PrimeFaces' in runtime ? runtime as PageRuntimeIdentity : undefined;
  const identity = page?.PrimeFaces;
  const version = page ? identity?.VERSION : runtime && typeof runtime === 'object' ? (runtime as PrimeFacesRuntimeIdentity).VERSION : runtime;
  const isKnown5 = typeof version === 'string' && /^5(?:\.|$)/.test(version);
  const hasLegacyRequestHook = version == null && typeof identity?.ajax?.Request?.handle === 'function'
    && typeof page?.jQuery?.ajaxPrefilter === 'function';
  if (isKnown5 || hasLegacyRequestHook) return new PrimeFacesAdapter();
  return new PrimeFaces15Adapter();
}
