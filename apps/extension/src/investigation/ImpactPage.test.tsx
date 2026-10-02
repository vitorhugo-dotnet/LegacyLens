// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { expect, test } from 'vitest';
import type { Command, CommandClient } from '../native/client.ts';
import { ImpactPage } from './ImpactPage.tsx';

const target = { id: 'table-orders', projectId: 'p1', revisionId: 'r1', artifactId: 'a1', path: 'reports/orders.jrxml', qualifiedName: 'orders', descriptor: '', kind: 'table' };
const source = { ...target, id: 'dao', artifactId: 'a2', path: 'src/OrderDao.java', qualifiedName: 'OrderDao.find', kind: 'method' };
const mapper = { ...source, id: 'mapper', qualifiedName: 'OrderDao.map' };

test('searches a target symbol, requests impact, and shows paths, evidence, and inferred-match diagnostics', async () => {
  const calls: Array<{ command: Command; payload: unknown }> = [];
  const client: CommandClient = { async request<T>(command: Command, payload: unknown) {
    calls.push({ command, payload });
    if (command === 'symbol.search') return { symbols: [target], total: 1, offset: 0, limit: 50, hasMore: false } as T;
    return { projectId: 'p1', revisionId: 'r1', symbols: [target, source, mapper], relations: [], paths: [{ sourceId: source.id, targetId: target.id, symbolIds: [source.id, mapper.id, target.id], relationIds: ['rel1', 'rel2'], evidenceIds: ['ev1'], inferred: true }], evidence: [{ id: 'ev1', kind: 'sql.table', source: 'FROM orders' }], diagnostics: [{ id: 'd1', code: 'impact.table_name_inferred', message: 'Nomes iguais; vínculo inferido.' }], depth: 8, offset: 0, limit: 50, total: 1, hasMore: false, truncated: false } as T;
  } };
  render(<ImpactPage client={client} projectId="p1" revisionId="r1" />);
  fireEvent.change(screen.getByLabelText('Símbolo-alvo'), { target: { value: 'orders' } });
  fireEvent.click(screen.getByRole('button', { name: 'Localizar símbolo' }));
  fireEvent.click(await screen.findByRole('button', { name: /Analisar impacto de orders/ }));
  expect(await screen.findByText('OrderDao.find → OrderDao.map → orders')).toBeVisible();
  expect(screen.getByText(/Caminho inferido/)).toBeVisible();
  expect(screen.getByText(/FROM orders/)).toBeVisible();
  expect(screen.getByText(/impact.table_name_inferred/)).toBeVisible();
  await waitFor(() => expect(calls.some(({ command, payload }) => command === 'impact.query' && (payload as { depth: number }).depth === 8)).toBe(true));
});
