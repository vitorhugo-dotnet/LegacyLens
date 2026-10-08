// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import type { Investigation, OpenResult, Page } from '@legacylens/contracts/src/protocol.ts';
import type { Command, CommandClient } from '../native/client.ts';
import { InvestigationPage } from './InvestigationPage.tsx';

const traceId = '0123456789abcdef0123456789abcdef';
const page = <T,>(items: T[], offset = 0, total = items.length): Page<T> => ({ items, offset, total, limit: 1, hasMore: offset + items.length < total });
const event = (id: string, sequence: number) => ({ projectId: 'p', traceId, producerId: 'browser', sequence, eventId: id, kind: 'jsf.click', occurredAt: '2026-10-01T00:00:00Z' });
const base = (offset = 0): Investigation => ({ project: { id: 'p', name: 'App', root: 'C:/App', createdAt: '2026-10-01T00:00:00Z' },
  trace: { id: traceId, projectId: 'p', startedAt: '2026-10-01T00:00:00Z', incomplete: true },
  agentStatus: { state: 'unknown' },
  events: page(offset ? [event('e2', 2)] : [event('e1', 1)], offset, 2),
  diagnostics: page(offset ? [] : [{ id: 'd', code: 'capture.sequence_gap', message: 'Gap', severity: 'warning', createdAt: '2026-10-01T00:00:00Z' }], offset, 1),
  symbols: page([], offset, 0),
  relations: page(offset ? [] : [{ id: 'static', fromId: 'source', kind: 'navigation', evidenceIds: ['ev'], resolution: 'dynamic', layer: 'static' }], offset, 1),
  evidence: page([], offset, 0) });

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

test('shows incomplete capture, reads later pages, and keeps static toggle separate from observed events', async () => {
  const calls: number[] = [];
  const client: CommandClient = { async request<T>(command: Command, payload: unknown) {
    if (command !== 'investigation.get') throw new Error('unexpected command');
    const offset = (payload as { offset: number }).offset;
    calls.push(offset);
    const result = base(offset);
    result.events = offset ? { ...page([event('e201', 201)], offset, 201), limit: 200 }
      : { ...page(Array.from({ length: 200 }, (_, i) => event(`e${i + 1}`, i + 1)), 0, 201), limit: 200 };
    return result as T;
  } };
  render(<InvestigationPage client={client} projectId="p" traceId={traceId} />);
  expect(await screen.findByText('Captura incompleta')).toBeVisible();
  expect(screen.getByText(/Agente: estado não confirmado/)).toBeVisible();
  await waitFor(() => expect(calls).toEqual([0, 200]));
  expect(screen.getByText(/201 de 201 eventos/)).toBeVisible();
  expect(screen.getByRole('button', { name: /e201/ })).toBeVisible();
  fireEvent.click(screen.getByLabelText('Relações estáticas'));
  expect(screen.queryByRole('button', { name: /navigation/ })).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: /e201/ })).toBeVisible();
});

test('retries an oversized native result with a smaller page', async () => {
  const limits: number[] = [];
  const client: CommandClient = { async request<T>(command: Command, payload: unknown) {
    if (command !== 'investigation.get') throw new Error('unexpected command');
    const limit = (payload as { limit: number }).limit;
    limits.push(limit);
    if (limit > 1) throw new Error('RESULT_TOO_LARGE: request a smaller page');
    return { ...base(), events: page([event('e1', 1)]), trace: { ...base().trace, incomplete: false } } as T;
  } };
  render(<InvestigationPage client={client} projectId="p" traceId={traceId} />);
  await waitFor(() => expect(screen.getByText(/1 de 1 eventos/)).toBeVisible());
  expect(limits[0]).toBe(200);
  expect(limits.at(-1)).toBe(1);
});

test('shows source-opening fallback when IntelliJ is unavailable', async () => {
  const location = { path: 'src/page.xhtml', line: 4, column: 1 };
  const client: CommandClient = { async request<T>(command: Command) {
    if (command === 'investigation.get') return { ...base(), events: page([]), trace: { ...base().trace, incomplete: false },
      symbols: page([{ id: 'source', projectId: 'p', revisionId: 'r', artifactId: 'a', path: location.path, qualifiedName: 'page', descriptor: '', kind: 'xhtml', location }]) } as T;
    if (command === 'location.open') return { opened: false, message: 'IntelliJ launcher is not configured', file: 'C:/App/src/page.xhtml', line: 4 } satisfies OpenResult as T;
    throw new Error('unexpected command');
  } };
  render(<InvestigationPage client={client} projectId="p" traceId={traceId} />);
  fireEvent.click(await screen.findByRole('button', { name: /^page$/ }));
  fireEvent.click(screen.getByRole('button', { name: 'Abrir no IntelliJ' }));
  expect(await screen.findByText(/IntelliJ launcher is not configured/)).toBeVisible();
  expect(screen.getByText(/C:\/App\/src\/page.xhtml:4/)).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: /navigation/ }));
  await waitFor(() => expect(screen.queryByText(/C:\/App\/src\/page.xhtml:4/)).not.toBeInTheDocument());
});

test('labels agent offline only when the status carries diagnostic evidence', async () => {
  const client: CommandClient = { async request<T>(command: Command) {
    if (command !== 'investigation.get') throw new Error('unexpected command');
    return { ...base(), agentStatus: { state: 'offline', evidenceDiagnosticId: 'agent-status-diagnostic' },
      diagnostics: page([{ id: 'agent-status-diagnostic', code: 'agent.offline', message: 'Heartbeat expired', severity: 'warning', createdAt: '2026-10-01T00:00:00Z' }]),
      events: page([]), trace: { ...base().trace, incomplete: false } } as T;
  } };
  render(<InvestigationPage client={client} projectId="p" traceId={traceId} />);
  expect(await screen.findByText(/Agente: offline/)).toBeVisible();
});

test('does not claim the agent is offline when status evidence is absent from loaded diagnostics', async () => {
  const client: CommandClient = { async request<T>(command: Command) {
    if (command !== 'investigation.get') throw new Error('unexpected command');
    return { ...base(), agentStatus: { state: 'offline', evidenceDiagnosticId: 'missing-diagnostic' }, events: page([]) } as T;
  } };
  render(<InvestigationPage client={client} projectId="p" traceId={traceId} />);
  expect(await screen.findByText(/Agente: estado não confirmado/)).toBeVisible();
});

test('renders a cross-producer fork as navigable nested causal branches', async () => {
  const investigation = { ...base(), events: page([]),
    symbols: page([
      { id: 'root', projectId: 'p', revisionId: '', artifactId: '', path: '', qualifiedName: 'click · producer browser-epoch · event click', descriptor: '', kind: 'observed-event' },
      { id: 'ajax', projectId: 'p', revisionId: '', artifactId: '', path: '', qualifiedName: 'ajax · producer browser-epoch · event ajax', descriptor: '', kind: 'observed-event' },
      { id: 'java', projectId: 'p', revisionId: '', artifactId: '', path: '', qualifiedName: 'request · producer java-epoch · event request', descriptor: '', kind: 'observed-java' },
    ]),
    relations: page([
      { id: 'edge-ajax', fromId: 'root', toId: 'ajax', kind: 'event.parent', evidenceIds: ['ajax'], resolution: 'resolved', layer: 'observed' },
      { id: 'edge-java', fromId: 'root', toId: 'java', kind: 'event.parent', evidenceIds: ['java'], resolution: 'resolved', layer: 'observed' },
    ]) } satisfies Investigation;
  const client: CommandClient = { async request<T>() { return investigation as T; } };
  render(<InvestigationPage client={client} projectId="p" traceId={traceId} />);
  const root = await screen.findByRole('button', { name: /click · producer browser-epoch/ });
  const ajax = screen.getByRole('button', { name: /ajax · producer browser-epoch/ });
  const java = screen.getByRole('button', { name: /request · producer java-epoch/ });
  expect(root.closest('li')).toContainElement(ajax);
  expect(root.closest('li')).toContainElement(java);
  fireEvent.click(java);
  expect(screen.getByRole('heading', { name: /request · producer java-epoch/ })).toBeVisible();
});

test('places missing and ambiguous parent gaps above their known children', async () => {
  const investigation = { ...base(), events: page([]),
    symbols: page([
      { id: 'missing-child', projectId: 'p', revisionId: '', artifactId: '', path: '', qualifiedName: 'missing-child', descriptor: '', kind: 'observed-event' },
      { id: 'ambiguous-child', projectId: 'p', revisionId: '', artifactId: '', path: '', qualifiedName: 'ambiguous-child', descriptor: '', kind: 'observed-event' },
    ]),
    relations: page([
      { id: 'missing', fromId: 'unknown-one', toId: 'missing-child', kind: 'event.parent_missing', evidenceIds: ['missing-child'], resolution: 'unresolved', layer: 'observed' },
      { id: 'ambiguous', fromId: 'unknown-two', toId: 'ambiguous-child', kind: 'event.parent_ambiguous', evidenceIds: ['ambiguous-child'], resolution: 'unresolved', layer: 'observed' },
    ]) } satisfies Investigation;
  const client: CommandClient = { async request<T>() { return investigation as T; } };
  render(<InvestigationPage client={client} projectId="p" traceId={traceId} />);
  const missing = await screen.findByRole('button', { name: /Pai desconhecido/ });
  const ambiguous = screen.getByRole('button', { name: /Pai ambíguo/ });
  expect(missing.closest('li')).toContainElement(screen.getByRole('button', { name: 'missing-child' }));
  expect(ambiguous.closest('li')).toContainElement(screen.getByRole('button', { name: 'ambiguous-child' }));
});

test('stops expanding a cyclic observed relation path', async () => {
  const investigation = { ...base(), events: page([]),
    symbols: page([
      { id: 'a', projectId: 'p', revisionId: '', artifactId: '', path: '', qualifiedName: 'event A', descriptor: '', kind: 'observed-event' },
      { id: 'b', projectId: 'p', revisionId: '', artifactId: '', path: '', qualifiedName: 'event B', descriptor: '', kind: 'observed-event' },
    ]),
    relations: page([
      { id: 'a-to-b', fromId: 'a', toId: 'b', kind: 'event.parent', evidenceIds: ['b'], resolution: 'resolved', layer: 'observed' },
      { id: 'b-to-a', fromId: 'b', toId: 'a', kind: 'event.parent', evidenceIds: ['a'], resolution: 'resolved', layer: 'observed' },
    ]) } satisfies Investigation;
  const client: CommandClient = { async request<T>() { return investigation as T; } };
  render(<InvestigationPage client={client} projectId="p" traceId={traceId} />);
  expect(await screen.findByText(/Ciclo de relação observado/)).toBeVisible();
  expect(screen.getByRole('button', { name: 'event A' }).closest('li')).toContainElement(screen.getByRole('button', { name: 'event B' }));
});

test('shows canonical revision mismatch with an empty indexed symbol snapshot', async () => {
  const investigation = { ...base(), indexedRevisionId: 'revision-A', symbols: page([]),
    events: page([{ ...event('java', 1), applicationRevision: 'revision-B' }]) } satisfies Investigation;
  const client: CommandClient = { async request<T>() { return investigation as T; } };
  render(<InvestigationPage client={client} projectId="p" traceId={traceId} />);
  expect(await screen.findByText(/Versão incompatível/)).toBeVisible();
  expect(screen.getByText(/Índice exibido: revisão revision-A/)).toBeVisible();
});

test('ignores a stale source-open completion after selecting another node', async () => {
  let resolveOpen!: (value: OpenResult) => void;
  const opened = new Promise<OpenResult>((resolve) => { resolveOpen = resolve; });
  const locationA = { path: 'a.xhtml', line: 1, column: 1 };
  const locationB = { path: 'b.xhtml', line: 2, column: 1 };
  const investigation = { ...base(), events: page([]), symbols: page([
    { id: 'a', projectId: 'p', revisionId: 'r', artifactId: 'a', path: 'a.xhtml', qualifiedName: 'A', descriptor: '', kind: 'xhtml', location: locationA },
    { id: 'b', projectId: 'p', revisionId: 'r', artifactId: 'b', path: 'b.xhtml', qualifiedName: 'B', descriptor: '', kind: 'xhtml', location: locationB },
  ]) } satisfies Investigation;
  const client: CommandClient = { async request<T>(command: Command) {
    return (command === 'location.open' ? await opened : investigation) as T;
  } };
  render(<InvestigationPage client={client} projectId="p" traceId={traceId} />);
  fireEvent.click(await screen.findByRole('button', { name: 'A' }));
  fireEvent.click(screen.getByRole('button', { name: 'Abrir no IntelliJ' }));
  fireEvent.click(screen.getByRole('button', { name: 'B' }));
  await act(async () => { resolveOpen({ opened: false, message: 'Old source', file: 'C:/App/a.xhtml', line: 1 }); await opened; });
  await waitFor(() => expect(screen.getByText('Fonte: b.xhtml:2')).toBeVisible());
  expect(screen.queryByText(/Old source/)).not.toBeInTheDocument();
});

test('keeps the investigation graph usable when the explanation provider fails', async () => {
  const oneEvent = event('e1', 1);
  const investigation = { ...base(), trace: { ...base().trace, incomplete: false }, events: page([oneEvent]),
    evidence: page([{ id: 'ev1', kind: 'observed-event', source: 'private source is not sent' }]) } satisfies Investigation;
  const client: CommandClient = { async request<T>(command: Command) {
    if (command === 'investigation.get') return investigation as T;
    if (command === 'explanation.preview') return { previewId: 'pv', expiresAt: '2026-10-08T12:05:00Z', destination: 'https://provider.example/v1/explanations', providerAvailable: true,
      package: { question: 'What happened?', deploymentRevisions: [], traceIncomplete: false,
        evidence: [{ id: 'ev1', kind: 'observed-event' }], limitations: ['Source text is excluded.'] } } as T;
    if (command === 'explanation.generate') throw new Error('EXPLANATION_FAILED: provider unavailable');
    throw new Error(`unexpected command: ${command}`);
  } };
  render(<InvestigationPage client={client} projectId="p" traceId={traceId} />);
  const eventButton = await screen.findByRole('button', { name: 'jsf.click · e1' });
  fireEvent.change(screen.getByLabelText('Pergunta'), { target: { value: 'What happened?' } });
  fireEvent.click(screen.getByRole('checkbox', { name: /ev1/ }));
  fireEvent.click(screen.getByRole('button', { name: 'Pré-visualizar envio' }));
  fireEvent.click(await screen.findByRole('checkbox', { name: /Autorizo enviar/ }));
  fireEvent.click(screen.getByRole('button', { name: 'Enviar e gerar explicação' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('EXPLANATION_FAILED');
  expect(eventButton).toBeVisible();
  fireEvent.click(eventButton);
  expect(screen.getByText('Produtor browser, sequência 1, evento e1')).toBeVisible();
});
