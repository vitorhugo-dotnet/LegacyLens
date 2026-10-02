import { useState } from 'react';
import type { SearchResult } from '@legacylens/contracts/src/protocol.ts';
import type { CommandClient } from '../native/client.ts';

const pageSize = 50;

export function SearchPage({ client, projectId, revisionId }: { client: CommandClient; projectId: string; revisionId: string }) {
  const [text, setText] = useState('');
  const [offset, setOffset] = useState(0);
  const [result, setResult] = useState<SearchResult>();
  const [error, setError] = useState('');
  const search = async (nextOffset = 0) => {
    const query = text.trim();
    if (!projectId || !revisionId || !query) return;
    setError(''); setOffset(nextOffset); setResult(undefined);
    try {
      const value = await client.request<SearchResult>('symbol.search', { projectId, revisionId, text: query, offset: nextOffset, limit: pageSize });
      setResult(value);
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Falha ao buscar símbolos.'); }
  };
  return <main className="analysis-page">
    <h2>Buscar no código indexado</h2>
    {!revisionId && <p role="status">Indexe o projeto para selecionar uma revisão pesquisável.</p>}
    <form onSubmit={(event) => { event.preventDefault(); void search(0); }}>
      <label>Buscar símbolo <input value={text} onChange={(event) => setText(event.target.value)} /></label>
      <button type="submit" disabled={!revisionId || !text.trim()}>Buscar</button>
    </form>
    {error && <p role="alert">{error}</p>}
    {result && <>
      <p>{result.total === 0 ? 'Nenhum símbolo encontrado.' : `${result.total} símbolos encontrados · revisão ${revisionId}`}</p>
      <ul>{result.symbols.map((symbol) => <li key={symbol.id}>
        <strong>{symbol.qualifiedName}</strong> <small>{symbol.kind} · {symbol.path}{symbol.location ? `:${symbol.location.line}` : ''}</small>
      </li>)}</ul>
      <div className="page-controls"><button disabled={offset === 0} onClick={() => void search(Math.max(0, offset - pageSize))}>Anterior</button><span>{offset + 1}–{Math.min(offset + result.symbols.length, result.total)} de {result.total}</span><button disabled={!result.hasMore} onClick={() => void search(offset + pageSize)}>Próxima</button></div>
    </>}
  </main>;
}
