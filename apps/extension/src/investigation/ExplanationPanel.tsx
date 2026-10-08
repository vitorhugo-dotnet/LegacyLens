import { useState } from 'react';
import type { Evidence, ExplanationPreview, ExplanationResult } from '@legacylens/contracts/src/protocol.ts';
import type { CommandClient } from '../native/client.ts';

export function ExplanationPanel({ client, projectId, traceId, evidence }: {
  client: CommandClient; projectId: string; traceId: string; evidence: Evidence[];
}) {
  const [question, setQuestion] = useState('');
  const [selectedEvidence, setSelectedEvidence] = useState<string[]>([]);
  const [preview, setPreview] = useState<ExplanationPreview>();
  const [consent, setConsent] = useState(false);
  const [result, setResult] = useState<ExplanationResult>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const invalidatePreview = () => { setPreview(undefined); setConsent(false); setResult(undefined); setError(''); };
  const toggleEvidence = (id: string, checked: boolean) => {
    invalidatePreview();
    setSelectedEvidence((current) => checked
      ? current.length < 20 ? [...current, id] : current
      : current.filter((value) => value !== id));
  };
  const createPreview = async () => {
    setError(''); setResult(undefined); setPreview(undefined); setConsent(false); setBusy(true);
    try {
      const value = await client.request<ExplanationPreview>('explanation.preview', { projectId, traceId, question, evidenceIds: selectedEvidence });
      setPreview(value);
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Não foi possível preparar a pré-visualização.'); }
    finally { setBusy(false); }
  };
  const send = async () => {
    if (!preview || !preview.providerAvailable || !consent) return;
    const approvedPreview = preview;
    setPreview(undefined); setConsent(false); setError(''); setResult(undefined); setBusy(true);
    try {
      const generated = await client.request<ExplanationResult>('explanation.generate', { previewId: approvedPreview.previewId, consent: true });
      setResult(generated);
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'O provedor não concluiu a solicitação. Faça um novo preview para tentar novamente.'); }
    finally { setBusy(false); }
  };

  return <section className="explanation-panel" aria-label="Explicação assistida">
    <h2>Explicação assistida</h2>
    <p>A pré-visualização é local. Nada será enviado até você revisar o pacote, marcar o consentimento e escolher enviar.</p>
    <label htmlFor="explanation-question">Pergunta</label>
    <textarea id="explanation-question" maxLength={2000} disabled={busy} value={question} onChange={(event) => { invalidatePreview(); setQuestion(event.target.value); }} />
    <fieldset>
      <legend>Escolha de 1 a 20 evidências</legend>
      {evidence.length === 0 && <p>Nenhuma evidência carregada.</p>}
      {evidence.map((item) => <label className="explanation-evidence" key={item.id}>
        <input type="checkbox" disabled={busy} checked={selectedEvidence.includes(item.id)} onChange={(event) => toggleEvidence(item.id, event.target.checked)} />
        <span>{item.id} · {item.kind}{item.location ? ` · ${item.location.path}:${item.location.line}` : ''}</span>
      </label>)}
    </fieldset>
    <button disabled={busy || !question.trim() || selectedEvidence.length === 0} onClick={() => void createPreview()}>
      {busy && !preview ? 'Preparando…' : 'Pré-visualizar envio'}
    </button>
    {preview && <div className="explanation-preview">
      <h3>Destino</h3><p>{preview.destination}</p>
      {!preview.providerAvailable && <p role="status">Nenhum provedor configurado no core. A busca e o grafo continuam disponíveis.</p>}
      <p>Preview válido até {new Date(preview.expiresAt).toLocaleString()} · uso único</p>
      <h3>Pacote exato que seria enviado</h3>
      <pre aria-label="Pacote exato que seria enviado">{JSON.stringify(preview.package, null, 2)}</pre>
      {preview.providerAvailable && <>
        <label className="explanation-consent"><input type="checkbox" checked={consent} onChange={(event) => setConsent(event.target.checked)} />
          Autorizo enviar este pacote ao destino mostrado acima.</label>
        <button disabled={busy || !consent} onClick={() => void send()}>{busy ? 'Enviando…' : 'Enviar e gerar explicação'}</button>
      </>}
    </div>}
    {error && <p role="alert">{error}</p>}
    {result && <div className="explanation-result" aria-label="Resultado da explicação">
      <h3>Afirmações</h3>
      {result.claims.map((claim, index) => <article key={`${index}-${claim.text}`}>
        <p>{claim.text}</p><p><strong>{claim.confidence === 'supported' ? 'Apoiada por evidência' : 'Hipótese'}</strong></p>
        <p>Evidências: {claim.evidenceIds.length ? claim.evidenceIds.join(', ') : 'Nenhuma referência'}</p>
      </article>)}
      <h3>Limitações</h3><ul>{result.limitations.map((limitation, index) => <li key={`${index}-${limitation}`}>{limitation}</li>)}</ul>
    </div>}
  </section>;
}
