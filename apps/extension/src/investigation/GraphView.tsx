import type { Event, Relation, Symbol } from '@legacylens/contracts/src/protocol.ts';

export type Selection = { kind: 'event'; value: Event } | { kind: 'symbol'; value: Symbol } | { kind: 'relation'; value: Relation };

export function GraphView({ events, symbols, relations, showStatic, onSelect }: {
  events: Event[]; symbols: Symbol[]; relations: Relation[]; showStatic: boolean; onSelect(value: Selection): void;
}) {
  const visible = relations.filter((relation) => showStatic || relation.layer !== 'static');
  const producers = [...new Set(events.map((event) => event.producerId))];
  const names = new Map(symbols.map((symbol) => [symbol.id, symbol.qualifiedName]));
  return <section aria-label="Grafo da investigação" className="graph-view">
    <h2>Caminhos observados</h2>
    {producers.map((producer) => <section key={producer} className="producer-branch">
      <h3>Produtor {producer}</h3>
      <ul>{events.filter((event) => event.producerId === producer).map((event) => <li key={`${event.projectId}:${event.traceId}:${event.producerId}:${event.eventId}`}>
        <button onClick={() => onSelect({ kind: 'event', value: event })}>{event.kind} · {event.eventId}</button>
        {event.parentEventId && <small> ← pai {event.parentEventId}</small>}
        {event.metadata?.stackGap && <small> · lacuna {event.metadata.stackGap}</small>}
      </li>)}</ul>
    </section>)}
    <h2>Símbolos</h2>
    <ul>{symbols.map((symbol) => <li key={symbol.id}><button onClick={() => onSelect({ kind: 'symbol', value: symbol })}>{symbol.qualifiedName}</button> <small>{symbol.kind}</small></li>)}</ul>
    <h2>Relações</h2>
    <ul>{visible.map((relation) => <li key={relation.id}><button onClick={() => onSelect({ kind: 'relation', value: relation })}>
      {names.get(relation.fromId) ?? relation.fromId} → {relation.toId ? names.get(relation.toId) ?? relation.toId : 'destino desconhecido'} · {relation.kind} · {relation.layer} · {relation.resolution}
    </button></li>)}</ul>
  </section>;
}
