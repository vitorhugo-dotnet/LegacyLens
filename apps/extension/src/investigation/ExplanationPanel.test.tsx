// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, expect, test } from 'vitest';
import type { Evidence } from '@legacylens/contracts/src/protocol.ts';
import type { Command, CommandClient } from '../native/client.ts';
import { ExplanationPanel } from './ExplanationPanel.tsx';

const selectedEvidence: Evidence[] = [
  { id: 'e1', kind: 'java-method', source: "SELECT * FROM orders WHERE note='private-literal'", location: { path: 'src/OrdersDao.java', line: 12, column: 4 } },
];
const preview = { previewId: 'preview-1', expiresAt: '2026-10-08T12:05:00Z', destination: 'https://provider.example/v1/explanations', providerAvailable: true,
  package: { question: 'What writes the order?', indexedRevisionId: 'rev1', deploymentRevisions: ['rev1'], traceIncomplete: false,
    evidence: [{ id: 'e1', kind: 'java-method', path: 'src/OrdersDao.java', line: 12, column: 4 }], limitations: ['Source text is excluded.'] } };

afterEach(() => cleanup());

test('previews exact data and destination locally, then sends only after explicit consent', async () => {
  const calls: Array<{ command: Command; payload: unknown }> = [];
  const client: CommandClient = { async request<T>(command: Command, payload: unknown) {
    calls.push({ command, payload });
    if (command === 'explanation.preview') return preview as T;
    if (command === 'explanation.generate') return { claims: [{ text: 'The DAO writes the order.', evidenceIds: ['e1'], confidence: 'supported' }], limitations: ['Confirm against the deployed revision.'] } as T;
    throw new Error('unexpected command');
  } };
  render(<ExplanationPanel client={client} projectId="p1" traceId="0123456789abcdef0123456789abcdef" evidence={selectedEvidence} />);
  fireEvent.change(screen.getByLabelText('Pergunta'), { target: { value: 'What writes the order?' } });
  fireEvent.click(screen.getByRole('checkbox', { name: /e1/ }));
  fireEvent.click(screen.getByRole('button', { name: 'Pré-visualizar envio' }));
  expect(await screen.findByText(preview.destination)).toBeVisible();
  expect(within(screen.getByLabelText('Pacote exato que seria enviado')).getByText(/src\/OrdersDao\.java/)).toBeVisible();
  expect(screen.queryByText(/private-literal/)).not.toBeInTheDocument();
  expect(calls.map((call) => call.command)).toEqual(['explanation.preview']);
  const consent = screen.getByRole('checkbox', { name: /Autorizo enviar/ });
  fireEvent.click(consent);
  fireEvent.click(screen.getByRole('button', { name: 'Enviar e gerar explicação' }));
  expect(await screen.findByText('The DAO writes the order.')).toBeVisible();
  expect(calls.map((call) => call.command)).toEqual(['explanation.preview', 'explanation.generate']);
  expect(calls[1]?.payload).toEqual({ previewId: 'preview-1', consent: true });
});

test('provider failure stays in the explanation panel and does not retry automatically', async () => {
  let attempts = 0;
  const client: CommandClient = { async request<T>(command: Command) {
    if (command === 'explanation.preview') return preview as T;
    if (command === 'explanation.generate') { attempts++; throw new Error('Provider request failed.'); }
    throw new Error('unexpected command');
  } };
  render(<ExplanationPanel client={client} projectId="p1" traceId="0123456789abcdef0123456789abcdef" evidence={selectedEvidence} />);
  fireEvent.change(screen.getByLabelText('Pergunta'), { target: { value: 'What writes the order?' } });
  fireEvent.click(screen.getByRole('checkbox', { name: /e1/ }));
  fireEvent.click(screen.getByRole('button', { name: 'Pré-visualizar envio' }));
  fireEvent.click(await screen.findByRole('checkbox', { name: /Autorizo enviar/ }));
  fireEvent.click(screen.getByRole('button', { name: 'Enviar e gerar explicação' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Provider request failed.');
  await waitFor(() => expect(attempts).toBe(1));
  expect(attempts).toBe(1);
});
