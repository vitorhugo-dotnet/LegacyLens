import type { ReactNode } from 'react';
import type { Event, Relation, Symbol } from '@legacylens/contracts/src/protocol.ts';

export type Selection = { kind: 'event'; value: Event } | { kind: 'symbol'; value: Symbol } | { kind: 'relation'; value: Relation };

export function GraphView({ events, symbols, relations, showStatic, onSelect }: {
  events: Event[]; symbols: Symbol[]; relations: Relation[]; showStatic: boolean; onSelect(value: Selection): void;
}) {
  const observed = symbols.filter((symbol) => symbol.kind.startsWith('observed-'));
  const byID = new Map(observed.map((symbol) => [symbol.id, symbol]));
  const children = new Map<string, Relation[]>();
  const incoming = new Set<string>();
  const gaps = new Map<string, Relation[]>();
  for (const relation of relations) {
    if (relation.layer !== 'observed' || !relation.toId || !byID.has(relation.toId)) continue;
    if (relation.resolution === 'resolved' && byID.has(relation.fromId)) {
      const outgoing = children.get(relation.fromId) ?? [];
      outgoing.push(relation);
      children.set(relation.fromId, outgoing);
      incoming.add(relation.toId);
    } else if (relation.resolution === 'unresolved') {
      const unresolved = gaps.get(relation.toId) ?? [];
      unresolved.push(relation);
      gaps.set(relation.toId, unresolved);
    }
  }
  const rendered = new Set<string>();
  const renderNode = (symbol: Symbol, path: Set<string>): ReactNode => {
    if (path.has(symbol.id)) return <li key={`cycle:${symbol.id}`}>Ciclo de relação observado; expansão interrompida.</li>;
    rendered.add(symbol.id);
    const nextPath = new Set(path); nextPath.add(symbol.id);
    const outgoing = children.get(symbol.id) ?? [];
    return <li key={symbol.id}>
      <button onClick={() => onSelect({ kind: 'symbol', value: symbol })}>{symbol.qualifiedName}</button>
      <small> {symbol.kind}</small>
      {outgoing.length > 0 && <ul>{outgoing.map((relation) => <li key={relation.id}>
        <button onClick={() => onSelect({ kind: 'relation', value: relation })}>Relação observada: {relation.kind}</button>
        <ul>{renderNode(byID.get(relation.toId!)!, nextPath)}</ul>
      </li>)}</ul>}
    </li>;
  };
  const roots = observed.filter((symbol) => !incoming.has(symbol.id));
  const renderRoot = (symbol: Symbol) => {
    const unresolved = gaps.get(symbol.id) ?? [];
    if (unresolved.length === 0) return renderNode(symbol, new Set());
    return <li key={`gap:${symbol.id}`}>
      {unresolved.map((relation) => <button key={relation.id} onClick={() => onSelect({ kind: 'relation', value: relation })}>
        {relation.kind === 'event.parent_ambiguous' ? 'Pai ambíguo' : relation.kind === 'event.parent_missing' ? 'Pai desconhecido' : 'Pai não confirmado'}
      </button>)}
      <ul>{renderNode(symbol, new Set())}</ul>
    </li>;
  };
  const branches = roots.map(renderRoot);
  for (const symbol of observed) if (!rendered.has(symbol.id)) branches.push(renderRoot(symbol));
  const staticRelations = relations.filter((relation) => relation.layer === 'static');
  const names = new Map(symbols.map((symbol) => [symbol.id, symbol.qualifiedName]));
  const producers = [...new Set(events.map((event) => event.producerId))];
  return <section aria-label="Grafo da investigação" className="graph-view">
    <h2>Caminhos observados</h2>
    <ul>{branches}</ul>
    <h2>Eventos por produtor</h2>
    {producers.map((producer) => <section key={producer} className="producer-branch">
      <h3>Produtor {producer}</h3>
      <ul>{events.filter((event) => event.producerId === producer).map((event) => <li key={`${event.projectId}:${event.traceId}:${event.producerId}:${event.eventId}`}>
        <button onClick={() => onSelect({ kind: 'event', value: event })}>{event.kind} · {event.eventId}</button>
        {event.parentEventId && <small> ← pai {event.parentEventId}</small>}
        {event.metadata?.stackGap && <small> · lacuna {event.metadata.stackGap}</small>}
      </li>)}</ul>
    </section>)}
    <h2>Símbolos indexados</h2>
    <ul>{symbols.filter((symbol) => !symbol.kind.startsWith('observed-')).map((symbol) => <li key={symbol.id}><button onClick={() => onSelect({ kind: 'symbol', value: symbol })}>{symbol.qualifiedName}</button> <small>{symbol.kind}</small></li>)}</ul>
    {showStatic && <><h2>Relações estáticas</h2><ul>{staticRelations.map((relation) => <li key={relation.id}><button onClick={() => onSelect({ kind: 'relation', value: relation })}>
      {names.get(relation.fromId) ?? relation.fromId} → {relation.toId ? names.get(relation.toId) ?? relation.toId : 'destino desconhecido'} · {relation.kind} · {relation.resolution}
    </button></li>)}</ul></>}
  </section>;
}
