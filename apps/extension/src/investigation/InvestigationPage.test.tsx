// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
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
