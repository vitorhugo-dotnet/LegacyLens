import { useEffect, useState } from 'react';
import type { Event, Investigation, Page } from '@legacylens/contracts/src/protocol.ts';
import type { CommandClient } from '../native/client.ts';
import { EvidencePanel } from './EvidencePanel.tsx';
import { GraphView, type Selection } from './GraphView.tsx';

type Collections = Pick<Investigation, 'events' | 'diagnostics' | 'symbols' | 'relations' | 'evidence'>;
type Loaded = { investigation: Investigation; complete: boolean; error?: string };

function merge<T>(old: Page<T>, next: Page<T>, identity: (item: T) => string): Page<T> {
  const items = new Map(old.items.map((item) => [identity(item), item]));
  for (const item of next.items) items.set(identity(item), item);
  return { items: [...items.values()], offset: 0, limit: next.limit, total: Math.max(old.total, next.total), hasMore: next.hasMore };
}

const eventIdentity = (event: Event) => `${event.projectId}\0${event.traceId}\0${event.producerId}\0${event.eventId}`;
const entityIdentity = (entity: { id: string }) => entity.id;

export async function loadInvestigation(client: CommandClient, projectId: string, traceId: string): Promise<Loaded> {
  let offset = 0;
  let limit = 200;
  let loaded: Investigation | undefined;
  for (;;) {
    let current: Investigation;
    try { current = await client.request<Investigation>('investigation.get', { projectId, traceId, offset, limit }); }
    catch (error) {
      if (error instanceof Error && error.message.includes('RESULT_TOO_LARGE') && limit > 1) { limit = Math.max(1, Math.floor(limit / 2)); continue; }
      if (loaded) return { investigation: loaded, complete: false, error: error instanceof Error ? error.message : 'Falha ao carregar página.' };
      throw error;
    }
    if (!loaded) loaded = current;
    else {
      const merged: Collections = {
        events: merge(loaded.events, current.events, eventIdentity),
        diagnostics: merge(loaded.diagnostics, current.diagnostics, entityIdentity),
        symbols: merge(loaded.symbols, current.symbols, entityIdentity),
        relations: merge(loaded.relations, current.relations, entityIdentity),
        evidence: merge(loaded.evidence, current.evidence, entityIdentity),
      };
      loaded = { ...current, ...merged, trace: { ...current.trace, incomplete: loaded.trace.incomplete === true || current.trace.incomplete === true } };
    }
    const aggregate = loaded!;
    const pages = [current.events, current.diagnostics, current.symbols, current.relations, current.evidence];
    if (pages.some((page) => page.offset !== offset || !Array.isArray(page.items) || page.limit < 1)) return { investigation: aggregate, complete: false, error: 'Página nativa inválida.' };
    if (!pages.some((page) => page.hasMore)) {
      const complete = [aggregate.events, aggregate.diagnostics, aggregate.symbols, aggregate.relations, aggregate.evidence].every((page) => page.items.length >= page.total);
      return { investigation: aggregate, complete };
    }
    offset += limit;
    if (offset > 1_000_000_000) return { investigation: aggregate, complete: false, error: 'Limite de paginação atingido.' };
  }
}

export function InvestigationPage({ client, projectId, traceId }: { client: CommandClient; projectId: string; traceId: string }) {
  const [loaded, setLoaded] = useState<Loaded>();
  const [error, setError] = useState('');
  const [showStatic, setShowStatic] = useState(true);
  const [selection, setSelection] = useState<Selection>();
  useEffect(() => {
    let active = true;
    setLoaded(undefined); setSelection(undefined); setError('');
    void loadInvestigation(client, projectId, traceId).then((value) => { if (active) setLoaded(value); }, (cause: unknown) => { if (active) setError(cause instanceof Error ? cause.message : 'Investigação indisponível.'); });
    return () => { active = false; };
  }, [client, projectId, traceId]);
  if (error) return <p role="alert">{error}</p>;
  if (!loaded) return <p role="status">Carregando investigação…</p>;
  const { investigation, complete } = loaded;
  const { events, symbols, relations, diagnostics, evidence, trace } = investigation;
  const indexedRevisions = new Set(symbols.items.filter((symbol) => symbol.revisionId).map((symbol) => symbol.revisionId));
  const declaredRevisions = new Set(events.items.filter((event) => event.applicationRevision).map((event) => event.applicationRevision));
  const mismatch = diagnostics.items.some((diagnostic) => diagnostic.code === 'source.version_mismatch')
    || (indexedRevisions.size === 1 && declaredRevisions.size > 0
      && [...indexedRevisions].every((revision) => revision.startsWith('revision-'))
      && [...declaredRevisions].every((revision) => revision?.startsWith('revision-'))
      && [...declaredRevisions].some((revision) => !indexedRevisions.has(revision!)));
  const agent = investigation.agentStatus;
  const agentEvidenceLoaded = !!agent.evidenceDiagnosticId && diagnostics.items.some((diagnostic) => diagnostic.id === agent.evidenceDiagnosticId);
  return <main className="investigation-page">
    <header><h1>Investigação · {investigation.project.name}</h1><p>Trace {trace.id}</p></header>
    {(trace.incomplete || !complete) && <strong role="status">Captura incompleta</strong>}
    {!trace.endedAt && <p>Captura em andamento; os totais podem mudar.</p>}
    {loaded.error && <p role="alert">Página incompleta: {loaded.error}</p>}
    <p>{events.items.length} de {events.total} eventos · {symbols.items.length} de {symbols.total} símbolos · {relations.items.length} de {relations.total} relações · {evidence.items.length} de {evidence.total} evidências · {diagnostics.items.length} de {diagnostics.total} diagnósticos</p>
    <p>Agente: {agent.state === 'offline' && agentEvidenceLoaded ? 'offline (diagnóstico confirmado)' : agent.state === 'online' && agentEvidenceLoaded ? 'online (diagnóstico confirmado)' : 'estado não confirmado'}</p>
    <p>{mismatch ? 'Versão incompatível: revisão declarada diverge do índice.' : 'Correspondência entre implantação e fonte não confirmada.'}</p>
    <label><input type="checkbox" checked={showStatic} onChange={(event) => setShowStatic(event.target.checked)} />Relações estáticas</label>
    <div className="investigation-layout">
      <GraphView events={events.items} symbols={symbols.items} relations={relations.items} showStatic={showStatic} onSelect={setSelection} />
      <EvidencePanel selection={selection} diagnostics={diagnostics.items} evidence={evidence.items} client={client} projectId={projectId} />
    </div>
  </main>;
}
