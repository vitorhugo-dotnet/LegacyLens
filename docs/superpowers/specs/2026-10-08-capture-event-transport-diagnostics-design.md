# LegacyLens: diagnóstico e transporte de eventos de captura

## Estado

Spec de arquitetura para localizar a perda do evento `jsf.click` e documentar seu redesenho. A instrumentação localizou no CI a ausência de confirmação entre content script e background. O transporte `chrome.runtime.Port` foi implementado conforme o plano de follow-up; aguarda a execução dos E2E no CI Windows.

## Objetivo

Fazer com que uma interação selecionada na página seja confirmada no host nativo e apareça na investigação, com evidência clara de cada fronteira do fluxo. Se o transporte atual for identificado como causa, substituí-lo por um fluxo com confirmação explícita, correlação e recuperação de desconexões.

O trabalho deve separar três fatos que hoje se confundem durante a depuração: o content script observou o clique, o background aceitou o evento, e o host persistiu o evento na sessão esperada.

## Contexto atual

O projeto usa uma extensão Chrome MV3, um content script no mundo isolado, um script no mundo principal para instrumentar Ajax/rede, um service worker de background, `NativeClient`, o host nativo local e o core LegacyLens.

O fluxo atual para o clique é:

```text
DOM click
  → chooseElement() no content script
  → dispatch de legacylens:select para o mundo principal
  → chrome.runtime.sendMessage({ type: capture.event, kind: jsf.click, ... })
  → listener do background valida aba, origem, sessão e payload
  → CaptureController.record()
  → NativeClient.request('trace.ingest', ...)
  → host/core persiste o evento
  → investigation.get consulta o trace
```

`legacylens:select` é um evento de diagnóstico visual entre mundos JavaScript da página e da extensão. Ele não confirma que `capture.event` chegou ao background nem que `trace.ingest` foi persistido. O script `page.content.ts` instrumenta eventos do navegador; ele não tem acesso às APIs de extensão para encaminhar eventos diretamente ao background.

O background confirma `capture.begin` antes da interação. O clique selecionado gera um `eventId` de 16 caracteres hexadecimais, e o background exige esse identificador para aceitar `jsf.click`. `CaptureController` serializa gravações por aba e envia um evento por comando `trace.ingest`.

Há duas saídas atualmente silenciosas que precisam de atenção na instrumentação: o listener do background retorna sem resposta quando não consegue validar `sender.tab.id` ou uma origem HTTP(S); e `ask()` tipa a resposta como `T`, mas não rejeita `undefined` nem confirma `accepted: true`. Portanto, uma mensagem sem resposta explícita pode não gerar aviso no content script. Isso é uma hipótese concreta a verificar no próximo trace, não uma causa já comprovada.

## Evidências observadas

Os resultados abaixo vêm dos traces e jobs de GitHub Actions. Em todas as execuções listadas, o job `Required checks (same commit)` passou; os E2E de navegador falharam.

| Commit em `main` | Execução | Evidência e falha |
| --- | --- | --- |
| `c333286` | [37795107658](https://github.com/vitorhugo-dotnet/LegacyLens/actions/runs/37795107658) | `capture.start` e o handshake `legacylens:start` ocorreram. A página mudou o texto do botão após o clique, mas o teste não observou `legacylens:select` no prazo. O E2E moderno também não encontrou `jsf.click`. |
| `2c26536` | [37796025459](https://github.com/vitorhugo-dotnet/LegacyLens/actions/runs/37796025459) | Com a detecção do alvo sem `instanceof`, o teste passou pela asserção de `legacylens:select`, mas `investigation.get` continuou retornando zero eventos para o trace DOM-only. O E2E moderno não encontrou `jsf.click`. |
| `e87e04c` | [37796685079](https://github.com/vitorhugo-dotnet/LegacyLens/actions/runs/37796685079) | A tentativa de obter a origem da aba como fallback não mudou o resultado: o E2E legado continuou sem evento persistido e o moderno continuou sem `jsf.click`. |
| `1381b09` | [37818722260](https://github.com/vitorhugo-dotnet/LegacyLens/actions/runs/37818722260) | O E2E esperou 45 s pela persistência. Para o mesmo clique, registrou `content.selection.accepted`, `content.send.started` e `content.send.timeout` após 35.002 ms; nenhum `background.receive` apareceu e a consulta continuou vazia. Trace `d39d8ec05173320e645946807567c37b`, evento `03b573e39396ae5f`. Os checks requeridos passaram; os E2E legado e Jakarta falharam. |

As falhas de `2c26536` e `e87e04c` mostravam que a seleção chega ao evento `legacylens:select`, mas não localizavam a perda. O run `1381b09` acrescentou a evidência correlacionada: o helper do content iniciou `chrome.runtime.sendMessage`, não recebeu ACK nem NACK e expirou após 35.002 ms; o background não registrou recebimento para o mesmo trace/evento. A falha fica localizada na fronteira one-shot content→background (entrega ou ciclo de vida da resposta), antes da validação e do host. O evento não apareceu em `investigation.get`.

O gate da Fase 1 está atendido. A candidata `Port` descrita na Fase 2 tem um plano de implementação separado em `docs/superpowers/plans/2026-10-08-capture-port-reliability.md`; a mudança de arquitetura ainda aguarda aprovação desse plano.

A validação local de TypeScript e o build da extensão passaram para as alterações anteriores. A execução E2E da fixture depende de PowerShell/Windows e a autoridade final de validação é o CI Windows.

## Tentativas anteriores e o que ensinaram

1. O fluxo de teste inicialmente acionava comandos por mensagens aninhadas ao service worker e ficava bloqueado durante `worker.evaluate`/`tabs.sendMessage`.
2. Foram adicionados limites de espera e marcadores de fase para separar conexão com o host, listagem de projetos, início da captura e entrega ao content script. Os marcadores mostraram que a rota de mensagens usada pelo fixture era instável.
3. O fixture passou a chamar um driver global de teste no service worker. Esse driver é habilitado somente em builds de fixture e continua usando o mesmo `CaptureController`, a injeção dos scripts e os comandos reais `capture.begin`/`capture.end`. Isso permitiu avançar o teste até a interação da página.
4. A verificação `instanceof HTMLElement` não reconhecia o alvo do clique em todos os contextos usados pelo Chrome. A seleção estrutural do nó fez `legacylens:select` aparecer no teste, mas não restaurou a persistência do evento.
5. O fallback de `sender.url` para `sender.tab.url` não resolveu a falta do evento. A origem continua sendo validada contra a origem da sessão, mas essa hipótese não deve ser tratada como causa confirmada.

As tentativas anteriores reduziram o problema até depois da seleção do DOM. A instrumentação posterior observou explicitamente as fronteiras seguintes; o resultado está na tabela acima.

## Abordagens consideradas

1. **Instrumentar o transporte one-shot atual (concluída).** Os registros correlacionados provaram que o content script inicia `sendMessage`, mas não recebe resposta em 35 segundos e o background não registra recebimento.
2. **Usar uma conexão `chrome.runtime.Port` (implementada; CI Windows pendente).** O content script estabelece uma conexão com o background, negocia a aba/sessão, envia eventos com identificador estável e recebe ACK/NACK após o host aceitar `trace.ingest`. Uma fila FIFO limitada e até três reconexões retransmitem eventos não confirmados usando os mesmos IDs; a deduplicação do core torna o replay idempotente.
3. **Mover captura/encaminhamento para mais código de página.** Não selecionada: o mundo principal não dispõe das APIs `chrome.runtime`; ainda precisaria de uma ponte de eventos para o content script e aumentaria a superfície de mensagens não confiáveis. Não remove as fronteiras que precisam ser diagnosticadas.

## Fase 1: instrumentação do transporte atual

### Correlação

Usar `traceId`, `tabId` e o `eventId` já criado para o clique como correlação. Cada registro deve identificar o estágio, resultado e duração. Não registrar HTML, texto de campos, conteúdo de requisições, tokens, cookies ou metadados da página que não sejam necessários à investigação.

### Fronteiras a observar

1. **Content script, seleção:** confirmar criação do evento e valores de correlação antes da chamada a `chrome.runtime.sendMessage`.
2. **Content → background:** registrar início do envio e resultado recebido. Diferenciar resposta válida com `accepted: true`, resposta `undefined`/sem listener, NACK explícito, rejeição da promessa e timeout. Não tratar resposta ausente como sucesso nem descartar silenciosamente erro em `capture.event`.
3. **Entrada do background:** registrar recebimento de `capture.event`, existência do `sender.tab.id`, URL/origem efetivamente usada, comparação com a origem da sessão e validação de `sessionId`, `eventId`, tipo e metadata. Registrar também cada saída antecipada do listener com um código seguro e uma resposta explícita quando for seguro responder. O registro não deve expor o payload completo.
4. **Background → host:** registrar início e fim de `CaptureController.record`/`trace.ingest`, duração, resultado ou código de erro. Distinguir mensagem inválida, sessão inexistente, Native Messaging indisponível, timeout e rejeição do core.
5. **Persistência → consulta:** confirmar que o host aceitou o evento e que `investigation.get` retorna o mesmo `eventId` no mesmo `traceId`. Se a API não permitir confirmar por ID, consultar tipo e identidade do evento sem relaxar a validação do teste.

Os sinais de diagnóstico devem ficar restritos ao build de fixture/teste ou usar logging de desenvolvimento controlado. Nenhum detalhe privado da aplicação deve aparecer no console de produção.

### Resultado da instrumentação

O run `1381b09` classificou a primeira falha na fronteira de entrega/resposta content→background. Após a implementação inicial do Port, o run `37821931816` confirmou que os checks unitários e o Go core passaram, mas os dois E2E falharam durante `capture.begin` com `Illegal invocation`, antes de a captura iniciar. A inspeção local atribuiu isso ao vínculo incorreto dos timers globais no cliente; o callback agora chama `globalThis.setTimeout/clearTimeout` pelo receiver correto. A correção aguarda nova execução no CI. Para o trace one-shot original:

- `content.selection.accepted` e `content.send.started` foram registrados;
- nenhum `background.receive` foi registrado;
- `content.send.timeout` terminou após 35.002 ms;
- `investigation.get` não retornou o evento.

O evento não chegou à validação do background, ao host ou à consulta. A resposta one-shot não foi confiável neste caminho; o redesenho com Port foi planejado separadamente.

## Fase 2: redesenho condicional com `Port`

O gatilho foi atendido pelo run `1381b09`. O usuário aprovou a execução direta do plano `docs/superpowers/plans/2026-10-08-capture-port-reliability.md`; a implementação local está concluída e espera a validação dos E2E no CI Windows.

### Protocolo proposto

- O content script abre `chrome.runtime.connect({ name: 'legacylens.capture.v1' })` quando a captura é armada.
- O background valida o remetente, associa o port a `tabId`, origem e `sessionId`, e responde `ready` ou `error` antes do primeiro evento.
- O content script envia envelopes tipados com versão, `traceId`, `eventId`, tipo de evento e metadata limitada. O controller mantém a sequência do produtor no host e preserva o `eventId` durante replay.
- O background valida novamente cada envelope. Só responde `ack` depois que o host confirmou `trace.ingest`; erros retornam `nack` com código seguro e correlação.
- Após queda do port, o content script pode reconectar e reenviar eventos pendentes com os mesmos IDs. O background/core deve garantir idempotência ou detectar duplicatas antes de ativar retransmissão.
- Ao encerrar a captura, o content script drena ACKs por até 35 segundos e informa se restaram eventos; o background marca a lacuna e encerra a sessão. Fechamento de aba e navegação de documento fecham os Ports, marcam a captura incompleta e encerram a sessão sem abrir investigação. Após restart do service worker, o content script reconecta e reproduz eventos pendentes com os mesmos IDs.

### Limites e riscos

`Port` não resolve falhas entre background e host nativo. Ele adiciona estado de conexão, ACKs, fila pendente, retransmissão e regras de duplicação; esse custo só se justifica se a Fase 1 atribuir a causa ao transporte content→background. Mensagens vindas do mundo principal continuam não confiáveis, e a validação de origem/sessão permanece no background.

## Critérios de aceite

### Fase 1

- Um único clique de fixture pode ser acompanhado pelo mesmo `traceId` e `eventId` desde a seleção até `investigation.get`.
- Cada fronteira registra sucesso, rejeição ou timeout com duração e código; não há falha silenciosa no caminho diagnosticado.
- Logs não contêm valores digitados, conteúdo da página, tokens ou cookies.
- O CI Windows diferencia falha de seleção, mensagem, validação, host e consulta.
- Se a falha não for do transporte one-shot, o teste passa após corrigir a causa real sem introduzir `Port`.

### Fase 2, se acionada

- O handshake do `Port` rejeita remetentes ou sessões incompatíveis antes de aceitar eventos.
- Um evento gera no máximo um registro persistido, mesmo após reenvio; a confirmação só ocorre depois do aceite pelo host.
- Desconexão, timeout e sessão encerrada produzem resultados observáveis e não deixam captura presa.
- Os E2E legacy e Jakarta verificam `jsf.click` e os eventos relacionados no trace correto; testes adicionais cobrem ACK, falha e reconexão.
- Builds de produção não habilitam driver global de fixture nem logging detalhado de diagnóstico.

## Fora de escopo

- Alterar a arquitetura do host nativo, protocolo externo do core ou a forma como `investigation.get` funciona sem evidência de que essas fronteiras causam a falha.
- Redesenhar a instrumentação Ajax/rede do mundo principal.
- Mudar o submenu de contexto, cadastro de projetos ou permissões de host.
- Tratar uma execução local sem PowerShell como substituta dos E2E Windows.

## Arquivos prováveis na implementação

- `apps/extension/entrypoints/content.ts`: sinais de envio/ACK e, se aprovado após instrumentação, cliente do `Port`.
- `apps/extension/entrypoints/background.ts`: recebimento, validação, correlação, respostas e lifecycle do port.
- `apps/extension/src/capture/session.ts`: resultado de gravação e confirmação do host; manter serialização e tratamento de perda.
- `apps/extension/src/native/client.ts`: logging de duração/resultado de `trace.ingest` sem payload sensível.
- `tests/e2e/legacy-click.spec.ts` e `tests/e2e/modern-click.spec.ts`: asserts correlacionados por ID e estágio.
- `tests/e2e/support/capture.ts`: apenas controles de fixture para iniciar/parar; o evento de usuário deve continuar usando o transporte de produção.

## Estado de validação da Fase 2

O plano implementa fila FIFO de até 100 eventos, até três reconexões, replay com o mesmo `eventId`, ACK após aceite do host, validação de duplicidade do core e limpeza no stop/fechamento/navegação. A primeira execução após o Port (`37821931816`) passou nos checks requeridos, inclusive Go, mas ambos os E2E encontraram `Illegal invocation` ao iniciar a captura. O cliente foi corrigido para vincular os timers ao global. Os testes unitários, typecheck, build e E2E discovery locais passaram antes da correção; é necessário repetir e aguardar o próximo CI Windows para confirmar os dois E2E.
