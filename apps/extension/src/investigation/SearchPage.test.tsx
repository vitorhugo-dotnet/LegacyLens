// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { expect, test } from 'vitest';
import type { Command, CommandClient } from '../native/client.ts';
import { SearchPage } from './SearchPage.tsx';

const symbol = { id: 's1', projectId: 'p1', revisionId: 'r1', artifactId: 'a1', path: 'src/OrderDao.java', qualifiedName: 'OrderDao.find', descriptor: '', kind: 'method', location: { path: 'src/OrderDao.java', line: 42, column: 3 } };

test('searches the indexed revision and displays source locations and result pagination', async () => {
  const calls: Array<{ command: Command; payload: unknown }> = [];
  const client: CommandClient = { async request<T>(command: Command, payload: unknown) {
    calls.push({ command, payload });
    return { symbols: [symbol], total: 1, offset: 0, limit: 50, hasMore: false } as T;
  } };
  render(<SearchPage client={client} projectId="p1" revisionId="r1" />);
  fireEvent.change(screen.getByLabelText('Buscar símbolo'), { target: { value: 'OrderDao' } });
  fireEvent.click(screen.getByRole('button', { name: 'Buscar' }));
  expect(await screen.findByText('OrderDao.find')).toBeVisible();
  expect(screen.getByText(/src\/OrderDao\.java:42/)).toBeVisible();
  await waitFor(() => expect(calls).toHaveLength(1));
  const request = calls[0]!;
  expect(request.command).toBe('symbol.search');
  expect(request.payload).toMatchObject({ projectId: 'p1', revisionId: 'r1', text: 'OrderDao', offset: 0, limit: 50 });
});
