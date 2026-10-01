import { useLayoutEffect, useRef, useState } from 'react';
import type { Diagnostic, Evidence, Location, OpenResult } from '@legacylens/contracts/src/protocol.ts';
import type { CommandClient } from '../native/client.ts';
import type { Selection } from './GraphView.tsx';

export function EvidencePanel({ selection, diagnostics, evidence, client, projectId }: {
  selection: Selection | undefined; diagnostics: Diagnostic[]; evidence: Evidence[]; client: CommandClient; projectId: string;
}) {
  const [openResult, setOpenResult] = useState<OpenResult | undefined>();
  const [openError, setOpenError] = useState('');
  const requestVersion = useRef(0);
  useLayoutEffect(() => {
    requestVersion.current += 1;
    setOpenResult(undefined);
    setOpenError('');
  }, [selection]);
  const relatedEvidence = selection?.kind === 'relation' ? evidence.filter((item) => selection.value.evidenceIds.includes(item.id)) : [];
  const location: Location | undefined = selection?.kind === 'symbol' || selection?.kind === 'relation' ? selection.value.location ?? relatedEvidence.find((item) => item.location)?.location : undefined;
  const open = async () => {
    if (!location) return;
    const version = ++requestVersion.current;
    setOpenResult(undefined);
    setOpenError('');
    try {
      const result = await client.request<OpenResult>('location.open', { projectId, location });
      if (version === requestVersion.current) setOpenResult(result);
    } catch (error) {
      if (version === requestVersion.current) setOpenError(error instanceof Error ? error.message : 'Não foi possível abrir a fonte.');
    }
  };
  return <aside aria-label="Evidência" className="evidence-panel">
    <h2>Evidência</h2>
    {!selection && <p>Selecione um evento, símbolo ou relação.</p>}
    {selection?.kind === 'event' && <>
      <h3>{selection.value.kind}</h3>
      <p>Produtor {selection.value.producerId}, sequência {selection.value.sequence}, evento {selection.value.eventId}</p>
      {selection.value.parentEventId && <p>Evento pai: {selection.value.parentEventId}</p>}
      {selection.value.metadata?.code && <p>Diagnóstico da extensão: {selection.value.metadata.code}</p>}
      {selection.value.applicationRevision && <p>Revisão declarada: {selection.value.applicationRevision}. Correspondência com fonte não confirmada.</p>}
      {selection.value.metadata?.['code.line_missing'] === 'true' && <p>Linha de depuração ausente.</p>}
      {selection.value.metadata?.frameChain && <p>Frames parciais: {selection.value.metadata.frameChain}</p>}
      {selection.value.metadata?.stackGap && <p>Lacuna de pilha: {selection.value.metadata.stackGap}</p>}
    </>}
    {selection?.kind === 'symbol' && <><h3>{selection.value.qualifiedName}</h3><p>Tipo: {selection.value.kind}</p></>}
    {selection?.kind === 'relation' && <><h3>{selection.value.kind}</h3><p>Camada: {selection.value.layer}; resolução: {selection.value.resolution}</p><p>Evidências: {selection.value.evidenceIds.join(', ')}</p>{relatedEvidence.map((item) => <p key={item.id}>{item.kind}: {item.source}</p>)}</>}
    {location ? <p>Fonte: {location.path}:{location.line}</p> : selection && <p>Local de origem não confirmado.</p>}
    {location && <button onClick={() => void open()}>Abrir no IntelliJ</button>}
    {openResult && !openResult.opened && <p role="status">{openResult.message}. {openResult.file && `${openResult.file}:${openResult.line ?? location?.line ?? 1}`}</p>}
    {openResult?.opened && <p role="status">{openResult.message || 'Fonte aberta.'}</p>}
    {openError && <p role="alert">{openError}. Use o caminho relativo mostrado acima.</p>}
    <h3>Diagnósticos</h3>
    <ul>{diagnostics.map((diagnostic) => <li key={diagnostic.id}>{diagnostic.code}: {diagnostic.message}</li>)}</ul>
  </aside>;
}
