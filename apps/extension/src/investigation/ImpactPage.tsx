import { useState } from 'react';
import type { ImpactResult, SearchResult, Symbol } from '@legacylens/contracts/src/protocol.ts';
import type { CommandClient } from '../native/client.ts';

const pageSize = 50;

export function ImpactPage({ client, projectId, revisionId }: { client: CommandClient; projectId: string; revisionId: string }) {
  const [text, setText] = useState('');
  const [depth, setDepth] = useState(8);
  const [matches, setMatches] = useState<Symbol[]>([]);
  const [target, setTarget] = useState<Symbol>();
  const [result, setResult] = useState<ImpactResult>();
  const [error, setError] = useState('');
  const [offset, setOffset] = useState(0);
  const locate = async () => {
    if (!projectId || !revisionId || !text.trim()) return;
    setError(''); setTarget(undefined); setResult(undefined); setMatches([]);
    try {
      const found = await client.request<SearchResult>('symbol.search', { projectId, revisionId, text: text.trim(), offset: 0, limit: pageSize });
      setMatches(found.symbols);
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Falha ao localizar o símbolo.'); }
  };
  const analyze = async (symbol: Symbol, nextOffset = 0) => {
    setTarget(symbol); setError(''); setOffset(nextOffset); setResult(undefined);
    try {
      const value = await client.request<ImpactResult>('impact.query', { projectId, revisionId, symbolId: symbol.id, depth, offset: nextOffset, limit: pageSize });
      setResult(value);
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Falha ao calcular impacto.'); }
  };
  const names = new Map(result?.symbols.map((symbol) => [symbol.id, symbol.qualifiedName]) ?? []);
  const evidence = new Map(result?.evidence.map((item) => [item.id, item]) ?? []);
  return <main className="analysis-page">
    <h2>Análise de impacto</h2>
    {!revisionId && <p role="status">Indexe o projeto para analisar uma revisão.</p>}
    <label>Símbolo-alvo <input value={text} onChange={(event) => setText(event.target.value)} /></label>
    <label>Profundidade <select value={depth} onChange={(event) => setDepth(Number(event.target.value))}>{Array.from({ length: 16 }, (_, index) => index + 1).map((value) => <option key={value} value={value}>{value}</option>)}</select></label>
    <button disabled={!revisionId || !text.trim()} onClick={() => void locate()}>Localizar símbolo</button>
    {error && <p role="alert">{error}</p>}
    {matches.length > 0 && <><h3>Escolha o alvo</h3><ul>{matches.map((symbol) => <li key={symbol.id}><button onClick={() => void analyze(symbol)} aria-label={`Analisar impacto de ${symbol.qualifiedName}`}>{symbol.qualifiedName}</button> <small>{symbol.kind} · {symbol.path}</small></li>)}</ul></>}
    {matches.length === 0 && text && !error && <p role="status">{target ? `Alvo: ${target.qualifiedName}` : 'Nenhum símbolo localizado ainda.'}</p>}
    {result && <section aria-label="Caminhos de impacto">
      <p>Impactos de {target?.qualifiedName} · revisão {result.revisionId} · profundidade {result.depth} · {result.total} caminhos</p>
      {result.truncated && <p role="status">Resultado truncado: há caminhos além da profundidade ou do limite explorado.</p>}
      <ul>{result.paths.map((path) => <li key={`${path.sourceId}:${path.relationIds.join(':')}`}>
        <strong>{path.symbolIds.map((id) => names.get(id) ?? id).join(' → ')}</strong>
        {path.inferred && <small> · Caminho inferido por correspondência do nome da tabela</small>}
        <small> · Evidências: {path.evidenceIds.map((id) => evidence.get(id)?.source ?? id).join('; ') || 'não disponíveis'}</small>
      </li>)}</ul>
      {result.paths.length === 0 && <p>Nenhum caminho estático confirmado neste limite.</p>}
      <ul>{result.diagnostics.map((diagnostic) => <li key={diagnostic.id} role="status">{diagnostic.code}: {diagnostic.message}</li>)}</ul>
      <div className="page-controls"><button disabled={offset === 0} onClick={() => target && void analyze(target, Math.max(0, offset - pageSize))}>Anterior</button><span>{result.total === 0 ? '0' : `${offset + 1}–${offset + result.paths.length}`} de {result.total}</span><button disabled={!result.hasMore} onClick={() => target && void analyze(target, offset + pageSize)}>Próxima</button></div>
    </section>}
  </main>;
}
