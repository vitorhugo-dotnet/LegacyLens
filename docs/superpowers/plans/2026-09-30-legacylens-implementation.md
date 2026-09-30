# LegacyLens Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Entregar o escopo integral do LegacyLens em fluxos verificáveis, da interação no navegador às evidências estáticas e observadas, com distribuição Windows e evolução modular.

**Architecture:** Monorepo com domínio e casos de uso Go independentes dos adaptadores. Extensão WXT, host nativo, analisadores e Java Agent comunicam-se por contratos versionados; grafos estáticos e traces preservam suas evidências e diferenças. Os marcos atravessam os componentes, pois sua utilidade depende da correlação entre eles.

**Tech Stack:** Go, SQLite com driver Go sem DLL externa, TypeScript/WXT/React, Java/Maven, Byte Buddy, JavaParser, parser SQL compatível com Java 8, parser JavaScript AST embarcado, CodeQL opcional, Vitest, Playwright e GitHub Actions.

**Spec:** [2026-09-30-legacylens-design.md](../specs/2026-09-30-legacylens-design.md), aprovada em 30/09/2026.

## Global Constraints

- Plataforma inicial: Windows 10 x64.
- Stack real informada: Java 8, JSF 2, PrimeFaces 5, MySQL 5 e JBoss/WildFly.
- O agente inicial deve executar em Java 8.
- Não há Go instalado na máquina de validação: os componentes serão distribuídos compilados.
- Código-fonte, navegador e JBoss/WildFly executam localmente na máquina de validação.
- Domínio e casos de uso independentes de WXT, JSF, PrimeFaces, CodeQL, JDBC, IntelliJ e armazenamento concreto.
- Um clique pode produzir zero a muitos caminhos; não inferir causalidade apenas por proximidade temporal.
- Relação estática não constitui evidência de execução; ausência de trace não prova impossibilidade.
- Suporte recente inclui a transição `javax` para `jakarta` e matriz explícita de versões/capacidades.
- Cookies, tokens, corpos HTTP e valores de parâmetros SQL ficam fora da coleta padrão.
- A aplicação permanece local por padrão; envio para IA requer operação explícita.
- Publicação no GitHub Releases depende das verificações obrigatórias da mesma revisão.
- CI aprovado e validação no sistema real são evidências distintas.
- TickTick é somente fonte de leitura. Não criar, modificar ou concluir suas tarefas.
- Não atualizar dependências sem pedido; dependências iniciais do projeto serão selecionadas e fixadas na tarefa que as introduz.
- Consultar CodeGraph antes de modificar símbolos existentes; usar contexto de edição, impacto e testes relacionados conforme as instruções do usuário.
- Inspecionar somente arquivos da tarefa, preservar mudanças alheias e usar validação local direcionada. O CI executa a matriz integral exigida pelo produto.

## Review Focus

1. Duas abas e polling concorrente: a investigação não agrega requisições sem vínculo causal. Testes nas tarefas 7, 8 e 16.
2. Caminhos Windows com espaços/acentos, junções e alteração durante indexação: localização segura e índice consistente. Testes nas tarefas 2, 3 e 12.
3. Redeploy no WildFly com classloaders distintos: não duplicar instrumentação nem carregar versão anterior como atual. Testes nas tarefas 6 e 17.
4. Conteúdo sensível em literal SQL, URL e exceção: nenhuma saída ou diagnóstico expõe o conteúdo. Testes nas tarefas 4, 6 e 19.
5. Desconexão, eventos repetidos/fora de ordem e limites: dados preservados, lacuna visível e aplicação sem bloqueio. Testes nas tarefas 4, 6, 9 e 16.

## Estado inicial e limites desta etapa

O Git tem um commit (`c161349`) e apenas a spec como arquivo rastreado. Não existe implementação a preservar ou símbolos de produto a consultar no CodeGraph nesta etapa. Este arquivo é o plano; nenhum dos arquivos de produto abaixo será criado antes da revisão e escolha do método de execução.

Executar em isolamento criado no início da implementação, conforme `superpowers:using-git-worktrees`; usar branch `codex/legacylens-product`. Não configurar remote, publicar, instalar no sistema real ou alterar a aplicação privada por inferência. A falta de remote não impede desenvolvimento e commits locais; registrar esse pré-requisito antes da primeira publicação.

## Estrutura de arquivos e responsabilidades

| Caminho | Responsabilidade |
| --- | --- |
| `core/go.mod`, `core/cmd/legacylens/`, `core/cmd/legacylens-host/` | Módulo Go e composição dos dois executáveis |
| `core/internal/domain/` | Entidades, identidade, causalidade e diagnósticos |
| `core/internal/application/` | Casos de uso e portas, DTOs independentes do transporte |
| `core/internal/adapters/` | Filesystem, SQLite, HTTP, framing nativo, analisadores, editor e IA |
| `contracts/` | JSON Schemas, exemplos válidos/inválidos e versão de protocolo |
| `packages/contracts/src/` | Tipos TypeScript e validadores do protocolo |
| `apps/extension/entrypoints/` | Background, seleção/captura e página de investigação |
| `apps/extension/src/` | Adaptadores PrimeFaces, contexto assíncrono, cliente e apresentação |
| `java/pom.xml`, `java/agent/`, `java/analyzer/` | Reactor Maven, agente instrumentador e worker estático |
| `analysis/codeql/` | Queries próprias e conversão de resultados; CLI externo não redistribuído |
| `fixtures/legacy/`, `fixtures/modern/`, `fixtures/static/` | Aplicações controladas e artefatos com resultados esperados |
| `tests/e2e/` | Testes de investigação com extensão, core e servidor |
| `scripts/` | Verificação, composição dos exemplos, empacotamento e instalação |
| `.github/workflows/` | CI e release condicionada ao resultado dos testes |
| `docs/user/`, `docs/validation/`, `docs/compatibility/` | Instalação, resultados e matriz de suporte |

Nomes de pacotes Java começam em `io.legacylens`. Módulo Go: `legacylens/core`. Pacote TypeScript compartilhado: `@legacylens/contracts`. Criar arquivos de um módulo somente na tarefa correspondente, evitando scaffold vazio de todo o produto.

## Decisões executáveis comuns

### Contratos e interfaces

Protocolo inicial `1`. Envelope JSON: `{protocolVersion, requestId, command, payload}`; resposta `{protocolVersion, requestId, result?, error?}`. Erro estruturado: `{code, message, diagnosticIds}`; não devolver stack ou comandos com credenciais. Comandos: `project.register`, `project.index`, `symbol.search`, `graph.explore`, `impact.query`, `capture.start`, `capture.stop`, `trace.ingest`, `investigation.get`, `location.open`, `explanation.preview`, `explanation.generate`. IDs de trace são 16 bytes aleatórios representados por 32 dígitos hexadecimais; spans usam 8 bytes/16 dígitos. A correlação HTTP usa `traceparent` com esse trace e span válidos, sem reutilizar um ID de símbolo.

No Go, `domain.ID` é um tipo string; campos de ID mantêm esse tipo. Entidades: `Project`, `Revision`, `Artifact`, `Symbol`, `Location`, `Relation`, `Evidence`, `Interaction`, `Trace`, `Event`, `Diagnostic`. `Location` usa caminho relativo ao projeto e linha/coluna base 1; referências sem localização são opcionais. `Relation` inclui `ID`, `FromID`, `ToID` opcional, `Kind`, `EvidenceIDs`, `Resolution` (`resolved`/`dynamic`/`unresolved`) e `Layer` (`static`/`observed`).

`Event` inclui `ProjectID`, `TraceID`, `ProducerID`, `Sequence`, `EventID`, `ParentEventID` opcional, `Kind`, `OccurredAt`, `ApplicationRevision` opcional e metadados permitidos. Identidade de evento: `(ProjectID, TraceID, ProducerID, EventID)`; sequência serve para detectar lacunas. Não inventar ordem total entre produtores.

DTOs da aplicação: `ProjectConfig`, `IndexRequest`, `IndexResult`, `AnalysisInput`, `AnalysisResult`, `Capability`, `SearchQuery`, `SearchResult`, `GraphQuery`, `GraphResult`, `CaptureRequest`, `CaptureSession`, `IngestResult`, `Investigation`, `OpenResult`, `ExplanationRequest`, `ExplanationResult`. Definições pertencem às tarefas 1/2/4; consumidores usam esses mesmos nomes.

Portas em `application/ports.go`:

```go
type Analyzer interface {
    Capabilities() []Capability
    Analyze(context.Context, AnalysisInput) (AnalysisResult, error)
}
type IndexStore interface {
    CommitIndex(context.Context, IndexResult) error
    Search(context.Context, SearchQuery) (SearchResult, error)
    Explore(context.Context, GraphQuery) (GraphResult, error)
}
type TraceStore interface {
    Append(context.Context, []domain.Event) (IngestResult, error)
    Load(context.Context, domain.ID, domain.ID) (Investigation, error)
}
type Editor interface {
    Open(context.Context, domain.Location) (OpenResult, error)
}
type Explainer interface {
    Generate(context.Context, ExplanationRequest) (ExplanationResult, error)
}
```

`IndexResult` carrega projeto/revisão, artefatos, símbolos, relações, evidências, diagnósticos e arquivos substituídos/excluídos. Atualizações substituem resultados por origem e invalidam dependências afetadas em transação. O armazenamento deve preservar a revisão usada por traces anteriores.

### Execução, armazenamento e limites

- Browser inicial escolhido para implementação: Chrome/Chromium Manifest V3. Outros browsers serão declarados não suportados até passarem por cenários próprios.
- Persistência SQLite por usuário, com migrações versionadas. Driver Go embarcado e sem dependência de instalação externa no Windows.
- API do core em loopback, porta dinâmica gravada em arquivo de descoberta protegido para o usuário. Autorização distinta para host/extensão e agente; a página web não recebe credenciais do core.
- HTTP: `POST /v1/commands`, `POST /v1/events` e `GET /v1/health`; limites e autorização aplicados antes do processamento. Arquivos de descoberta/configuração usam permissões por usuário e nunca aparecem em logs.
- Host `io.legacylens.host`: framing stdio com comprimento uint32 little-endian, JSON UTF-8. Limite interno de frame 512 KiB, paginação para resultados grandes e stdout reservado ao protocolo.
- Defaults novos deste plano: 4.096 eventos na fila do agente, 10.000 eventos por captura, captura de até 10 minutos, payload de até 512 KiB e página de até 200 resultados. Expor configuração e produzir diagnóstico quando um limite for atingido.
- Workers Java rodam por subprocesso com argumentos separados e protocolo JSONL; nenhum comando usa concatenação de shell. Worker estático usa Java já disponível e pode ser configurado separadamente do servidor.
- Novas bibliotecas e toolchains são fixadas ao introduzir cada módulo, com licença e compatibilidade verificadas e registradas em `docs/compatibility/toolchains.md`; sem intervalos dinâmicos ou `latest` em arquivos de build.

### Combinações controladas de referência

| Exemplo | Versões escolhidas para o plano | Estado antes da execução |
| --- | --- | --- |
| Antigo | Java 8; WildFly 10.0.0.Final; JSF 2.2 fornecido pelo servidor; PrimeFaces 5.3; MySQL 5.7.44; Connector/J 5.1.49 | Experimental |
| Recente | Java 21; WildFly 35.0.0.Final; Jakarta Faces 4.0 fornecido pelo servidor; PrimeFaces 15.0.0 com classifier `jakarta`; MySQL 8.4.0; Connector/J 8.4.0 | Experimental |

Essas combinações são exemplos de teste, não a identificação presumida do sistema real nem promessa de cobrir todas as versões atuais. Fixar também checksums/imagens por digest e registrar a versão JSF/Faces efetivamente carregada; não empacotar uma segunda implementação Faces no WAR. A montagem e os testes da tarefa 17 comprovam a combinação integral. Divergência detectada exige corrigir o plano/matriz antes de anunciar suporte.

CodeQL será configurado por caminho externo e usado somente quando o usuário confirmar que seu uso está autorizado pelos termos/licença aplicáveis. Sua ausência não impede o analisador básico. Não baixar nem embutir o CLI na release. Queries próprias podem ser distribuídas conforme sua licença. A tarefa 14 implementa extração manual com build configurado ou modo sem build quando aceito pela versão do CLI, registrando as diferenças de resolução.

### Regras de verificação e commits

Os comandos a seguir são destinados à execução futura, não foram executados ao escrever este plano. Go usa `go -C core test`; npm workspaces usa `npm test --workspace apps/extension -- ...`; Java usa `mvn -f java/pom.xml -pl <module> -am ...` com `-Dsurefire.failIfNoSpecifiedTests=false` apenas para módulos agregados sem o teste selecionado.

Cada tarefa termina em um commit dos arquivos explícitos da tarefa, após a validação dirigida passar: `git add -- <arquivos da tarefa>` e `git commit -m "<mensagem indicada>"`. Não usar `git add .`. Falhas de ambiente são registradas como validação incompleta.

## Marco A — Primeiro fluxo completo

### Task 1: Modelo compartilhado e contratos versionados

**Files — Create:** `contracts/protocol-v1.schema.json`, `contracts/domain-v1.schema.json`, `contracts/examples/valid-interaction.json`, `contracts/examples/invalid-relation.json`, `contracts/examples/invalid-version.json`, `core/go.mod`, `core/internal/domain/model.go`, `core/internal/domain/identity.go`, `core/internal/domain/model_test.go`, `core/internal/application/dto.go`, `core/internal/application/ports.go`, `package.json`, `package-lock.json`, `tsconfig.base.json`, `packages/contracts/package.json`, `packages/contracts/tsconfig.json`, `packages/contracts/vitest.config.ts`, `packages/contracts/src/protocol.ts`, `packages/contracts/src/protocol.test.ts`, `.gitignore`, `docs/compatibility/toolchains.md`.

**Interfaces:** Produces entidades/DTOs/portas acima; `domain.SymbolID(projectID, revisionID ID, path, qualifiedName, descriptor string) ID`; TypeScript `parseEnvelope(input: unknown): Envelope`. SQLite e frameworks não entram nesses pacotes.

- [ ] Escrever `TestSymbolIdentityIncludesProjectAndDescriptor`: `id(projectA,"load(int)") != id(projectB,"load(int)")` e `id(projectA,"load(int)") != id(projectA,"load(String)")`; testar eventos sem destino Java e relações sem localização.
- [ ] Escrever `protocol.test.ts`: `expect(parseEnvelope(valid).protocolVersion).toBe(1)`; versão 2 e relação sem evidência geram erro tipado, não coerção silenciosa.
- [ ] Executar `go -C core test ./internal/domain -run TestSymbolIdentity` e `npm test --workspace packages/contracts -- protocol.test.ts`; confirmar falha pelo comportamento ausente após instalar somente os módulos necessários.
- [ ] Implementar schemas, tipos, validadores e identidade; fixar toolchains/dependências iniciais verificadas e lockfiles. Gerar exemplos usados pelos dois lados. Root workspaces inclui `packages/*`, `apps/*` e `tests/e2e`; cada workspace adiciona seu script de teste ao ser introduzido.
- [ ] Reexecutar os testes e verificar importações: domínio/aplicação não importam `internal/adapters`; commit `feat: define domain and versioned contracts`.

### Task 2: Registro de projeto, índice mínimo e persistência

**Files — Create:** `core/internal/application/projects.go`, `core/internal/application/index.go`, `core/internal/application/index_test.go`, `core/internal/adapters/filesystem/source.go`, `core/internal/adapters/filesystem/source_test.go`, `core/internal/adapters/sqlite/store.go`, `core/internal/adapters/sqlite/migrations/001_initial.sql`, `core/internal/adapters/sqlite/store_test.go`, `core/internal/adapters/static/xhtml/analyzer.go`, `core/internal/adapters/static/xhtml/analyzer_test.go`, `fixtures/static/minimal/view.xhtml`.

**Interfaces:** Consumes task 1; produces `RegisterProject(ctx context.Context, config ProjectConfig) (domain.Project,error)` e `Indexer.Index(ctx context.Context, request IndexRequest) (IndexResult,error)`; SQLite implementa `IndexStore`. Scanner retorna artefatos com hash e revisão; analisador XHTML implementa `Analyzer`.

- [ ] Escrever `TestIndexXHTMLActionEvidence`: `action="#{orders.save}"` gera relação de ação com linha correta, evidência e destino dinâmico até Java ser resolvido. `TestProjectPathWithSpacesAndAccents` deve preservar `C:\Projetos\Gestão Legada`.
- [ ] Escrever `TestCommitIndexRollsBackOnFailure`: depois de falha simulada, a revisão anterior permanece íntegra; exclusão de diretórios configurados impede indexar artefatos deles.
- [ ] Executar `go -C core test ./internal/application ./internal/adapters/filesystem ./internal/adapters/sqlite ./internal/adapters/static/xhtml`; confirmar as falhas esperadas.
- [ ] Implementar registro, hashing, exclusões e armazenamento transacional com migração inicial; impedir fuga do projeto por `..`, link ou junção; XML não resolve entidades externas.
- [ ] Reexecutar testes; commit `feat: register projects and persist a minimal source index`.

### Task 3: Localizações, revisão implantada e IntelliJ

**Files — Create:** `core/internal/application/locations.go`, `core/internal/application/locations_test.go`, `core/internal/adapters/intellij/editor.go`, `core/internal/adapters/intellij/editor_test.go`, `docs/user/intellij.md`.

**Interfaces:** Consumes `Editor`, `Location`; produces `LocationService.Open(ctx context.Context, projectID domain.ID, location domain.Location) (OpenResult,error)` e `CompareRevision(indexed, deployed domain.Revision) string` (`confirmed`/`mismatch`/`unconfirmed`). Adaptador executa launcher configurado com lista de argumentos.

- [ ] Escrever `TestOpenLocationArguments`: caminho com espaços/acentos e linha 42 produz argumentos separados `--line`, `42`, caminho absoluto; `../outside.java` é rejeitado. `TestRevisionMissingIsUnconfirmed` nunca retorna `confirmed` para revisão ausente.
- [ ] Executar `go -C core test ./internal/application ./internal/adapters/intellij -run 'TestOpenLocation|TestRevision'`; confirmar falha.
- [ ] Implementar launcher do IntelliJ configurado pelo usuário, sem depender de API HTTP não documentada; retornar `OpenResult` com arquivo/linha para fallback quando launcher faltar. Comparação exige identificação verificável da implantação.
- [ ] Reexecutar os testes, registrar uso do launcher; commit `feat: open verified source locations in IntelliJ`.

### Task 4: Sessões, ingestão e sanitização

**Files — Create:** `core/internal/application/captures.go`, `core/internal/application/ingest.go`, `core/internal/application/ingest_test.go`, `core/internal/domain/sanitize.go`, `core/internal/domain/sanitize_test.go`, `core/internal/adapters/sqlite/traces.go`, `core/internal/adapters/sqlite/traces_test.go`.

**Interfaces:** Produces `CaptureService.Start(ctx context.Context, request CaptureRequest) (CaptureSession,error)`, `Stop(ctx context.Context, projectID, traceID domain.ID) error`, `Ingest(ctx context.Context, events []domain.Event) (IngestResult,error)`, `domain.SanitizeMetadata(input map[string]string) (map[string]string, []domain.Diagnostic)`. SQLite implementa `TraceStore`.

- [ ] Escrever `TestIngestIdempotentAndOutOfOrder`: o mesmo `EventID` duas vezes resulta em um evento; filho antes do pai conserva referência e lacuna até o pai chegar. `TestCaptureLimitMarksIncomplete` aplica 10.000 eventos com diagnóstico.
- [ ] Escrever `TestSanitizeMetadata`: token em querystring, cookie e exceção não aparece na saída; metadado não permitido é removido. Texto SQL inválido não é preservado como fallback bruto.
- [ ] Executar `go -C core test ./internal/application ./internal/domain ./internal/adapters/sqlite -run 'TestIngest|TestCapture|TestSanitize'`; confirmar falha.
- [ ] Implementar estado de sessão, relógio injetável, sanitização por allowlist, ingestão deduplicada e armazenamento com diagnósticos de sequência/limite. Eventos de outra sessão não são anexados por horário.
- [ ] Reexecutar testes; commit `feat: persist bounded captures with evidence and diagnostics`.

### Task 5: API local e host de Native Messaging

**Files — Create:** `core/cmd/legacylens/main.go`, `core/cmd/legacylens-host/main.go`, `core/internal/adapters/localapi/server.go`, `core/internal/adapters/localapi/server_test.go`, `core/internal/adapters/localapi/discovery_windows.go`, `core/internal/adapters/localapi/discovery_unix.go`, `core/internal/adapters/native/framing.go`, `core/internal/adapters/native/framing_test.go`, `core/internal/adapters/native/bridge.go`, `core/internal/adapters/native/bridge_test.go`.

**Interfaces:** Consumes tasks 1–4; produces `localapi.NewServer(services Services, auth AuthConfig) http.Handler`, `native.ReadFrame(r io.Reader, maxBytes uint32) (Envelope,error)`, `WriteFrame(w io.Writer, envelope Envelope) error` e `Bridge.Run(ctx context.Context, in io.Reader, out io.Writer) error`. `Services` compõe casos de uso, nunca lógica de domínio no handler.

- [ ] Escrever `TestFramePartialReadAndOversize`: leitura em fragmentos retorna envelope íntegro; frame acima de 512 KiB é recusado antes da alocação integral. `TestBridgeRejectsUnknownOrigin` recusa ID de extensão não provisionado.
- [ ] Escrever `TestAPIRejectsMissingTokenAndUnknownVersion`: acesso sem credencial retorna 401; protocolo incompatível retorna erro tipado; tokens/caminhos da configuração não aparecem no log.
- [ ] Executar `go -C core test ./internal/adapters/native ./internal/adapters/localapi`; confirmar falha.
- [ ] Implementar composição, loopback, descoberta por usuário, autenticação e framing. `--parent-window` não é tratado como identidade. CLI oferece `serve`, `status`, `project register`, `index` e `inspect --trace` para operação sem UI.
- [ ] Reexecutar testes e compilar os dois comandos; commit `feat: connect local core through an authenticated native host`.

### Task 6: Java Agent para HTTP, métodos e JDBC

**Files — Create:** `java/pom.xml`, `java/agent/pom.xml`, `java/agent/src/main/java/io/legacylens/agent/LegacyLensAgent.java`, `java/agent/src/main/java/io/legacylens/agent/Event.java`, `java/agent/src/main/java/io/legacylens/agent/AgentConfig.java`, `java/agent/src/main/java/io/legacylens/agent/TraceContext.java`, `java/agent/src/main/java/io/legacylens/agent/AgentTransport.java`, `java/agent/src/main/java/io/legacylens/agent/instrumentation/ServletAdvice.java`, `java/agent/src/main/java/io/legacylens/agent/instrumentation/MethodAdvice.java`, `java/agent/src/main/java/io/legacylens/agent/instrumentation/JdbcAdvice.java`, `java/agent/src/main/java/io/legacylens/agent/Sanitizer.java`, `java/agent/src/test/java/io/legacylens/agent/AgentFlowTest.java`, `java/agent/src/test/java/io/legacylens/agent/AgentFailureTest.java`.

**Interfaces:** Consumes protocol 1 and `/v1/events`; produces `premain(String options, Instrumentation instrumentation)`, `TraceContext.current(): TraceContext`, `AgentTransport.offer(Event event): boolean` e POJO Java `Event` com campos JSON do schema da tarefa 1. Configuração por arquivo inclui endpoint, credencial, allowlist de pacotes, projeto e identificação implantada. Não registra valores de arguments/return.

- [ ] Escrever `AgentFlowTest`: request com identificação válida captura servlet → método da aplicação → JDBC; exception gera evento de término sem mensagem sensível. JDBC aninhado do wrapper/driver não duplica o mesmo efeito.
- [ ] Escrever `AgentFailureTest`: core offline e fila de 4.096 cheia não bloqueiam request; emitirá diagnóstico de perdas ao recuperar conexão. Duas classes iguais em classloaders distintos preservam identidade de implantação.
- [ ] Executar `mvn -f java/pom.xml -pl agent -am -Dtest=AgentFlowTest,AgentFailureTest -Dsurefire.failIfNoSpecifiedTests=false test`; confirmar falha.
- [ ] Implementar Byte Buddy com runtime Java 8, bridge acessível aos classloaders instrumentados, exclusão do próprio agente e allowlist da aplicação. Separar instrumentação Servlet `javax`/`jakarta`; limpar contexto em `finally`; sanitizar SQL antes de enfileirar e excluir parâmetros. Incluir nome qualificado, descritor e localização de debug quando disponível, com ausência explícita quando não houver linha. O reactor Maven registra somente módulos existentes, começando por `agent`.
- [ ] Reexecutar testes em Java 8 e 21; verificar bytecode Java 8 e JAR sombreado com dependências relocadas; commit `feat: trace application methods and JDBC with a Java 8 agent`.

### Task 7: Extensão, seleção e captura PrimeFaces inicial

**Files — Create:** `apps/extension/package.json`, `apps/extension/tsconfig.json`, `apps/extension/vitest.config.ts`, `apps/extension/wxt.config.ts`, `apps/extension/entrypoints/background.ts`, `apps/extension/entrypoints/content.ts`, `apps/extension/entrypoints/page.content.ts`, `apps/extension/src/native/client.ts`, `apps/extension/src/capture/selection.ts`, `apps/extension/src/capture/session.ts`, `apps/extension/src/adapters/primefaces5.ts`, `apps/extension/src/adapters/primefaces5.test.ts`, `apps/extension/src/capture/session.test.ts`. **Modify:** `package-lock.json` ao instalar o novo workspace.

**Interfaces:** Consumes envelopes/task 5; produces `NativeClient.request<T>(command: Command, payload: unknown): Promise<T>`, `CaptureController.start(request: CaptureRequest): Promise<CaptureSession>` e `PrimeFacesAdapter.install(context: PageCaptureContext): () => void`. A página MAIN não recebe tokens; background possui a conexão nativa.

- [ ] Escrever `primefaces5.test.ts`: clique captura ID JSF do elemento e a chamada Ajax da ação; requisição de polling concorrente não recebe o identificador do clique. `session.test.ts` verifica isolamento entre abas e somente uma captura ativa por aba.
- [ ] Executar `npm test --workspace apps/extension -- primefaces5.test.ts session.test.ts`; confirmar falha.
- [ ] Implementar seleção visual, permissões por origem escolhida, captura próxima interação/elemento e hook MAIN para PrimeFaces 5. Propagar `traceparent` somente nas requisições same-origin vinculadas à captura, preservando headers existentes e comportamento da aplicação.
- [ ] Reexecutar testes; persistir estado necessário no background para sobreviver à suspensão e informar lacunas após reconexão; commit `feat: capture selected PrimeFaces interactions in the extension`.

### Task 8: JavaScript, contexto assíncrono e requests

**Files — Create:** `apps/extension/src/capture/async-context.ts`, `apps/extension/src/capture/async-context.test.ts`, `apps/extension/src/capture/network.ts`, `apps/extension/src/capture/network.test.ts`, `apps/extension/src/capture/stack.ts`, `apps/extension/src/capture/stack.test.ts`, `fixtures/static/browser/interaction.html`. **Modify:** `apps/extension/entrypoints/page.content.ts`.

**Interfaces:** Produces `withInteraction<T>(context: InteractionContext, fn: () => T): T`, `bindInteraction<T extends (...args: any[]) => any>(fn: T, context: InteractionContext): T`, `installNetworkCapture(context: PageCaptureContext): () => void` e `parseStack(stack: string): SourceFrame[]`. Consumes tarefa 7, não associa callbacks apenas porque ocorreram durante a captura.

- [ ] Escrever `async-context.test.ts`: clique A → timer → callback gera pai correto; polling já existente e clique B não herdam A; identidade de listener permite `removeEventListener` funcionar.
- [ ] Escrever `network.test.ts`: fetch e XHR mantêm resultado/exception original, request não vinculado e cross-origin não recebem header; `stack.test.ts` extrai cadeia `first → second → fetch` e marca stack ausente como lacuna.
- [ ] Executar `npm test --workspace apps/extension -- async-context.test.ts network.test.ts stack.test.ts`; confirmar falha.
- [ ] Implementar wrappers limitados ao contexto capturado, restauráveis, preservando `this`, arguments, retorno e exceptions; suportar timers, listeners e callbacks Promise. Frames observados e relações estáticas complementam funções que não admitem hook; nunca afirmar cobertura completa de closures sem observação.
- [ ] Reexecutar testes; commit `feat: preserve interaction causality through JavaScript and network effects`.

### Task 9: Investigação visual com evidências

**Files — Create:** `core/internal/application/investigations.go`, `core/internal/application/investigations_test.go`, `apps/extension/entrypoints/investigation/index.html`, `apps/extension/entrypoints/investigation/main.tsx`, `apps/extension/src/investigation/GraphView.tsx`, `apps/extension/src/investigation/EvidencePanel.tsx`, `apps/extension/src/investigation/InvestigationPage.tsx`, `apps/extension/src/investigation/InvestigationPage.test.tsx`, `apps/extension/src/investigation/styles.css`.

**Interfaces:** Produces `InvestigationService.Get(ctx context.Context, projectID, traceID domain.ID) (Investigation,error)` e `InvestigationPage({client,projectId,traceId})`; consumes tasks 2–8. Resposta paginada preserva diagnósticos e relações incompletas.

- [ ] Escrever `TestInvestigationSeparatesLayers`: relação estática continua estática mesmo quando há trace relacionado; sequência com lacuna retorna diagnóstico. UI: `expect(screen.getByText('Captura incompleta')).toBeVisible()` e toggle estático não altera eventos observados.
- [ ] Executar `go -C core test ./internal/application -run TestInvestigation` e `npm test --workspace apps/extension -- InvestigationPage.test.tsx`; confirmar falha.
- [ ] Implementar fluxo navegável, ramificações, seleção de nó/relação, painel de evidência, abrir no IntelliJ e fallback. Antes do analisador Java da tarefa 11, materializar símbolos observados a partir do evento e vincular localizações apenas a artefatos indexados cuja correspondência possa ser verificada; ausência de informação de debug fica visível. Mostrar versão incompatível, agente offline e correspondência não confirmada junto à investigação. Paginar resultados em vez de exceder frame nativo.
- [ ] Reexecutar testes; commit `feat: inspect correlated traces and source evidence`.

### Task 10: Exemplo antigo e primeira validação ponta a ponta

**Files — Create:** `fixtures/legacy/pom.xml`, `fixtures/legacy/src/main/java/io/legacylens/fixture/OrderBean.java`, `fixtures/legacy/src/main/java/io/legacylens/fixture/OrderService.java`, `fixtures/legacy/src/main/java/io/legacylens/fixture/OrderDao.java`, `fixtures/legacy/src/main/webapp/orders.xhtml`, `fixtures/legacy/src/main/webapp/WEB-INF/web.xml`, `fixtures/legacy/sql/schema.sql`, `fixtures/legacy/fixture-lock.json`, `scripts/fixtures/start-legacy.ps1`, `scripts/fixtures/stop.ps1`, `tests/e2e/package.json`, `tests/e2e/playwright.config.ts`, `tests/e2e/legacy-click.spec.ts`, `scripts/package-preview.ps1`, `docs/user/quickstart.md`. **Modify:** `package-lock.json` ao instalar o novo workspace.

**Interfaces:** Produces app com botão `saveOrder`, SQL em `orders`, revisão implantada verificável e cenário Playwright real com extensão/host/core/agente. Scripts usam health/readiness e encerramento por PID/processo que criaram, sem parar serviços alheios.

- [ ] Escrever `legacy-click.spec.ts`: clique gera JSF → `OrderBean.save` → `OrderService.save` → `OrderDao.insert` → tabela `orders`, evidências selecionáveis e dados sem parâmetros SQL; interação DOM sem request produz zero eventos Java.
- [ ] Executar `npm test --workspace tests/e2e -- legacy-click.spec.ts` com exemplo iniciado pelo script; falha deve indicar falta de comportamento, não ser aceite como falha de setup.
- [ ] Implementar exemplo controlado e scripts, artefatos/checksums fixados, configuração nativa para Chromium de teste e pacote preview com executáveis/JAR/extension. Não requer o sistema privado.
- [ ] Reexecutar cenário com cleanup em `finally`, timeout e logs sanitizados; registrar o resultado do ambiente controlado; commit `test: validate the first complete legacy interaction flow`.

## Marco B — Análise estática, busca e impacto

### Task 11: Worker Java/SQL e resolução de símbolos

**Files — Create:** `java/analyzer/pom.xml`, `java/analyzer/src/main/java/io/legacylens/analyzer/AnalyzerMain.java`, `java/analyzer/src/main/java/io/legacylens/analyzer/JavaAnalyzer.java`, `java/analyzer/src/main/java/io/legacylens/analyzer/SqlAnalyzer.java`, `java/analyzer/src/test/java/io/legacylens/analyzer/JavaAnalyzerTest.java`, `java/analyzer/src/test/java/io/legacylens/analyzer/SqlAnalyzerTest.java`, `core/internal/adapters/static/javaworker/analyzer.go`, `core/internal/adapters/static/javaworker/analyzer_test.go`, `fixtures/static/java/Overloads.java`, `fixtures/static/java/ReflectiveCalls.java`, `fixtures/static/sql/queries.sql`. **Modify:** `java/pom.xml` para registrar `analyzer`.

**Interfaces:** Worker consome JSONL `AnalysisInput`, emite `AnalysisResult`; JavaParser resolve símbolos com source roots/classpath configurados e gera diagnóstico de resolução ausente. `javaworker.New(config WorkerConfig) Analyzer` usa protocolo da tarefa 1.

- [ ] Escrever `JavaAnalyzerTest`: chamadas sobrecarregadas resolvem descritores diferentes; reflexão sem alvo conhecido gera `dynamic`; herança/interfaces conservam candidatos sem inventar dispatch. `SqlAnalyzerTest`: join, alias e backticks identificam tabela/coluna; SQL malformado gera diagnóstico sanitizado.
- [ ] Executar `mvn -f java/pom.xml -pl analyzer -am -Dtest=JavaAnalyzerTest,SqlAnalyzerTest -Dsurefire.failIfNoSpecifiedTests=false test` e `go -C core test ./internal/adapters/static/javaworker`; confirmar falha.
- [ ] Implementar parsers compatíveis com runtime Java 8 e sintaxe recente declarada; fixture de código Java 21 deve ser analisável sem compilar no Java 8. Se uma construção não for suportada pelo parser selecionado, marcar capacidade, não tokenizar com regex como se resolvida.
- [ ] Reexecutar testes, timeout/cancelamento do worker e paths com espaços; commit `feat: analyze Java call relationships and SQL dependencies`.

### Task 12: JavaScript AST, XHTML/EL e atualização incremental

**Files — Create:** `core/internal/adapters/static/javascript/analyzer.go`, `core/internal/adapters/static/javascript/analyzer_test.go`, `core/internal/adapters/static/el/resolver.go`, `core/internal/adapters/static/el/resolver_test.go`, `core/internal/application/incremental.go`, `core/internal/application/incremental_test.go`, `fixtures/static/browser/remote-command.xhtml`, `fixtures/static/browser/handlers.js`. **Modify:** XHTML analyzer, Indexer e SQLite das tarefas 2/4.

**Interfaces:** JS implementa `Analyzer` com AST embarcada e build Windows sem runtime Node; produz referências de funções, handlers, requests e remote commands. `ELResolver.Resolve(expression string, symbols []domain.Symbol) ([]domain.Relation,[]domain.Diagnostic)`; `Indexer.Update(ctx context.Context, request IndexRequest) (IndexResult,error)`.

- [ ] Escrever `TestResolveManagedBeanAndRemoteCommand`: `first → second → remoteSave → #{orders.save}` tem evidências por etapa; nomes em comentários/string sem chamada não geram chamada. `TestIncrementalDeleteAndChangedDependency`: renomear/remover método invalida relações antigas e mantém trace histórico.
- [ ] Escrever `TestIndexFileChangedDuringRead`: arquivo alterado entre hash/leitura é reprocessado ou gera diagnóstico, nunca revisão confirmada com conteúdo inconsistente.
- [ ] Executar `go -C core test ./internal/adapters/static/javascript ./internal/adapters/static/el ./internal/application -run 'TestResolve|TestIncremental|TestIndexFileChanged'`; confirmar falha.
- [ ] Implementar adapters JS/EL, resolução por evidência e transação incremental; se parser exigir CGO, compilação acontece no CI com toolchain fixada e lib embutida, sem runtime adicional na máquina alvo.
- [ ] Reexecutar testes; commit `feat: connect browser code and maintain incremental source relationships`.

### Task 13: JRXML, busca e análise de impacto

**Files — Create:** `core/internal/adapters/static/jrxml/analyzer.go`, `core/internal/adapters/static/jrxml/analyzer_test.go`, `core/internal/application/search.go`, `core/internal/application/impact.go`, `core/internal/application/impact_test.go`, `apps/extension/src/investigation/SearchPage.tsx`, `apps/extension/src/investigation/ImpactPage.tsx`, `apps/extension/src/investigation/ImpactPage.test.tsx`, `fixtures/static/reports/orders.jrxml`, `fixtures/static/reports/summary.jrxml`.

**Interfaces:** JRXML implementa `Analyzer`; `SearchService.Search(ctx context.Context, query SearchQuery) (SearchResult,error)`; `ImpactService.Query(ctx context.Context, query GraphQuery) (GraphResult,error)`. Resultado inclui caminho, evidências e diagnósticos, com limite de profundidade/paginação explícitos.

- [ ] Escrever `TestImpactTableReachesViewAndReport`: tabela `orders` retorna DAO, método, tela e JRXML por caminhos evidenciados. Ciclo e subreport dinâmico terminam com diagnóstico, sem travar ou afirmar destino desconhecido.
- [ ] Escrever `ImpactPage.test.tsx`: pesquisar classe/método/campo/tabela/endpoint/relatório mostra tipo e origem; resultados truncados mostram limite; selecionar caminho mostra suas evidências.
- [ ] Executar `go -C core test ./internal/adapters/static/jrxml ./internal/application -run 'TestImpact|TestJRXML'` e `npm test --workspace apps/extension -- ImpactPage.test.tsx`; confirmar falha.
- [ ] Implementar leitura JRXML moderno e legado com DTD externa desabilitada, query/fields/expressions/subreports; travessia reversa com controle de ciclos e índice de busca. Não compilar Jasper nem executar consultas do projeto para indexar.
- [ ] Reexecutar testes; commit `feat: investigate symbols and report source-backed change impact`.

### Task 14: Backend CodeQL opcional

**Files — Create:** `analysis/codeql/qlpack.yml`, `analysis/codeql/java/calls.ql`, `analysis/codeql/java/endpoints.ql`, `analysis/codeql/README.md`, `core/internal/adapters/static/codeql/analyzer.go`, `core/internal/adapters/static/codeql/analyzer_test.go`, `core/internal/adapters/static/codeql/testdata/calls.bqrs.json`, `docs/user/codeql.md`.

**Interfaces:** `codeql.New(config Config) Analyzer`, com caminho externo, modo de extração, build como lista de argumentos e confirmação explícita de uso autorizado. Consome `AnalysisInput`, converte resultados de queries/BQRS em `AnalysisResult`, preservando backend e evidência.

- [ ] Escrever `TestCodeQLUnavailableLeavesBasicAnalysisUsable`, `TestCodeQLRequiresExplicitEnablement` e `TestDecodeCallRowsKeepsLocation`: parser de resultados usa dados congelados e diagnóstico sem expor comando/segredos.
- [ ] Executar `go -C core test ./internal/adapters/static/codeql`; confirmar falha.
- [ ] Implementar detecção de versão/capacidade, criação de database configurada, execução das queries e decode JSON; timeout/cancelamento. Não usar SARIF de alertas como substituto de um grafo geral de chamadas.
- [ ] Reexecutar testes dirigidos; validar queries em fixture autorizada com CLI configurado. Sem ambiente autorizado, backend permanece experimental e validação é registrada como incompleta, sem impedir indexação básica; commit `feat: integrate an optional externally configured CodeQL backend`.

## Marco C — Cobertura de integrações e causalidade

### Task 15: Endpoints, Faces recente e identificação da implantação

**Files — Create:** `java/agent/src/main/java/io/legacylens/agent/instrumentation/FacesAdvice.java`, `java/agent/src/main/java/io/legacylens/agent/instrumentation/EndpointAdvice.java`, `java/agent/src/main/java/io/legacylens/agent/ApplicationIdentity.java`, `java/agent/src/main/java/io/legacylens/agent/ApplicationRevision.java`, `java/agent/src/test/java/io/legacylens/agent/EndpointFlowTest.java`, `apps/extension/src/adapters/primefaces15.ts`, `apps/extension/src/adapters/primefaces15.test.ts`, `fixtures/legacy/src/main/java/io/legacylens/fixture/OrderServlet.java`. **Modify:** registro de adaptadores e composição do agente.

**Interfaces:** `FacesAdapter.supports(DetectedRuntime runtime): boolean`; `ApplicationIdentity.describe(Class<?> applicationClass): ApplicationRevision`; browser adapters declaram namespace/versão e fallback com diagnóstico.

- [ ] Escrever `EndpointFlowTest`: request fetch → filter/servlet e Faces action/actionListener produzem múltiplos métodos com identidade implantada; recurso não instrumentado não recebe identidade confirmada por suposição.
- [ ] Escrever `primefaces15.test.ts`: remote command recente preserva comportamento e retorno Promise; versão não reconhecida informa suporte experimental em vez de selecionar PF5 silenciosamente.
- [ ] Executar Maven para `EndpointFlowTest` e Vitest para `primefaces15.test.ts`; confirmar falha.
- [ ] Implementar detecção e adapters `javax`/`jakarta`, correlação HTTP e extração de revisão do manifesto do WAR quando disponível; revisão ausente continua `unconfirmed`. Nenhuma modificação obrigatória no código privado para iniciar o agente.
- [ ] Reexecutar testes; commit `feat: extend traces across Faces and servlet integrations`.

### Task 16: Concorrência, dinâmica e recuperação de captura

**Files — Create:** `java/agent/src/main/java/io/legacylens/agent/instrumentation/AsyncAdvice.java`, `java/agent/src/test/java/io/legacylens/agent/AsyncContextTest.java`, `core/internal/application/reconciliation.go`, `core/internal/application/reconciliation_test.go`, `tests/e2e/causality.spec.ts`, `tests/e2e/recovery.spec.ts`. **Modify:** trace context/transporte, session background e UI.

**Interfaces:** `Reconciler.Reconcile(ctx context.Context, projectID, traceID domain.ID) (Investigation,error)` relaciona observado/estático por identidade verificável e nunca muda o tipo da evidência original; contexto assíncrono carrega pai somente em mecanismo instrumentado.

- [ ] Escrever `AsyncContextTest`: Runnable originado na request preserva contexto; tarefa pré-existente e tarefa seguinte no pool não o herdam; exception limpa contexto.
- [ ] Escrever E2E `causality.spec.ts`: dois cliques, timer, duas requests e polling geram árvores isoladas; método refletido executado aparece como observado sem converter todas as alternativas estáticas em executadas.
- [ ] Escrever `recovery.spec.ts`: desconectar agente/host, reiniciar background e exceder limite conservam dados, mostram captura incompleta e não congelam aplicação.
- [ ] Executar testes dirigidos de reconciliação/Maven e os dois specs Playwright; confirmar falhas.
- [ ] Implementar propagação para executores suportados, reconexão limitada, reenvio idempotente, finalização por tempo injetável e diagnósticos para mecanismos não cobertos; reexecutar testes; commit `feat: reconcile dynamic flows and recover partial captures`.

## Marco D — Compatibilidade, CI e distribuição

### Task 17: Matriz antiga/recente e redeploy real nos exemplos

**Files — Create:** `fixtures/modern/pom.xml`, `fixtures/modern/src/main/java/io/legacylens/fixture/OrderBean.java`, `fixtures/modern/src/main/java/io/legacylens/fixture/OrderService.java`, `fixtures/modern/src/main/java/io/legacylens/fixture/OrderDao.java`, `fixtures/modern/src/main/webapp/orders.xhtml`, `fixtures/modern/src/main/webapp/WEB-INF/web.xml`, `fixtures/modern/sql/schema.sql`, `fixtures/modern/fixture-lock.json`, `scripts/fixtures/start-modern.ps1`, `tests/e2e/modern-click.spec.ts`, `tests/e2e/redeploy.spec.ts`, `docs/compatibility/matrix.json`, `docs/compatibility/README.md`.

**Interfaces:** Mesmos eventos/contratos da fixture antiga; matriz registra versão exata detectada, capacidades, resultado, revisão testada e plataforma. `redeploy.spec.ts` implanta duas revisões e consulta identidade no trace.

- [ ] Escrever `modern-click.spec.ts`: cenário completo com stack recente e pesquisa/impacto mantém semântica do antigo. `redeploy.spec.ts`: revisão B não duplica eventos nem referencia classes da revisão A como implantação atual.
- [ ] Executar os specs em ambas as combinações; falhas de setup são corrigidas antes de julgar instrumentação. Registrar versão de JSF/Faces do runtime, não apenas a declarada no POM.
- [ ] Implementar fixture recente, detecção de versões/capacidades e isolamento de redeploy. Compatibilidade de bytecode/runtime dos JARs e dependências é verificada em Java 8/21.
- [ ] Reexecutar cenários, estabelecer baseline de tempo/eventos e degradar para diagnóstico quando runtime não identificado; commit `test: validate legacy and Jakarta compatibility matrices`.

### Task 18: GitHub Actions, pacote Windows e release condicionada

**Files — Create:** `.github/workflows/ci.yml`, `.github/workflows/release.yml`, `scripts/verify.ps1`, `scripts/package-release.ps1`, `scripts/install-native-host.ps1`, `scripts/uninstall-native-host.ps1`, `scripts/test-package.ps1`, `scripts/tests/Package.Tests.ps1`, `scripts/tests/NativeHost.Tests.ps1`, `scripts/tests/Workflow.Tests.ps1`, `packaging/native-host.template.json`, `packaging/release-manifest.schema.json`, `docs/user/windows-installation.md`. **Modify:** `apps/extension/wxt.config.ts` e quickstart.

**Interfaces:** Package produz ZIP Windows x64 com core, host, extensão unpacked, agente e worker Java, scripts, documentação, licenças e SHA-256. Manifest registra commit, versões, protocolo e verificações. Host HKCU usa ID explícito autorizado da extensão; chave pública da extensão pode fixar ID sem distribuir chave privada.

- [ ] Escrever Pester `Package.Tests.ps1`: pacote completo roda `legacylens.exe status` sem Go/Node, JAR carrega em Java 8 e 21; CodeQL CLI ausente. `NativeHost.Tests.ps1`: instalar/remover em caminho com espaços altera somente sua chave/manifesto e é idempotente.
- [ ] Escrever `Workflow.Tests.ps1`: jobs obrigatórios com resultado `failure`, `cancelled` ou `skipped` tornam publicação inelegível; assets correspondem ao mesmo SHA e tag; schemas e hashes íntegros são requeridos.
- [ ] Executar `pwsh -File scripts/verify.ps1 -Scope packaging` depois que o script existir; primeiro ciclo deve expor comportamentos ainda ausentes; não publicar para testar o gate.
- [ ] Implementar CI por push/PR: contratos, Go, TS, JAR Java 8/21, fixtures antiga/recente com MySQL e E2E, e smoke do pacote Windows. Fixar actions por SHA e dependências; timeout, health checks, cleanup e logs sanitizados. Release por tag `v*`, checkout do SHA da tag e jobs próprios que executam/reutilizam verificações desse mesmo SHA via workflow reutilizável; `publish` depende de todos os resultados obrigatórios `success`.
- [ ] Empacotar em runner Windows e validar sem toolchains no PATH; runner não é prova de Windows 10. Publicar ZIP/manifesto/checksums e notas que distinguem combinações testadas, experimental e sistema real; reexecutar validação dirigida; commit `ci: build tested Windows packages and gate GitHub releases`.

### Task 19: Validação no Windows 10 e ganho em manutenção

**Files — Create:** `scripts/collect-diagnostics.ps1`, `scripts/tests/Diagnostics.Tests.ps1`, `docs/validation/windows10-checklist.md`, `docs/validation/maintenance-comparison-template.md`, `docs/validation/result.schema.json`, `docs/user/jboss-agent.md`. **Modify:** matriz de compatibilidade somente com resultados efetivamente coletados.

**Interfaces:** `collect-diagnostics.ps1` gera estado/versionamento sanitizado, sem copiar fonte, configuração secreta ou trace completo automaticamente. Resultado de validação tem commit/release, Windows/build, Java/servidor/JSF/PF/MySQL/driver exatos, revisão do índice/implantação, cenários e limites.

- [ ] Escrever `Diagnostics.Tests.ps1`: URL com token, exceção com SQL literal e arquivo de credencial nunca aparecem no bundle. Resultado com cenário não executado não pode ter estado `passed`.
- [ ] Executar `pwsh -File scripts/verify.ps1 -Scope diagnostics`; confirmar falha dos comportamentos ausentes.
- [ ] Implementar instruções para `standalone.conf.bat` e modo de inicialização efetivamente usado, config-file do agente em diretório do usuário, coleta opt-in e checklist sem depender de Go. Registrar instalação, remoção, IntelliJ, captura sem request, fluxo completo e análise de impacto.
- [ ] Reexecutar os testes. Solicitar ao usuário execução dos passos que exigem a outra máquina e incorporar resultados reais quando fornecidos; não alegar validação privada concluída sem evidência. Comparar uma investigação manual e uma com o produto, medindo tempo e verificando os pontos encontrados.
- [ ] Commit `docs: provide real-system validation and maintenance measurements`; marco permanece com validação externa pendente até resultados serem registrados.

## Marco E — IA apoiada nas evidências

### Task 20: Explicações referenciadas e envio explícito

**Files — Create:** `core/internal/application/explanations.go`, `core/internal/application/explanations_test.go`, `core/internal/adapters/explainer/http.go`, `core/internal/adapters/explainer/http_test.go`, `contracts/explanation-v1.schema.json`, `apps/extension/src/investigation/ExplanationPanel.tsx`, `apps/extension/src/investigation/ExplanationPanel.test.tsx`, `docs/user/explanations.md`.

**Interfaces:** `ExplanationService.Preview(ctx context.Context, request ExplanationRequest) (ExplanationRequest,error)` constrói pacote mínimo sanitizado; `Generate(ctx context.Context, request ExplanationRequest) (ExplanationResult,error)` chama `Explainer` somente após consentimento para aquele pacote. Resultado inclui texto, `EvidenceIDs` por afirmação e limitações. Adaptador HTTP implementa contrato interno documentado, configurável para serviço local/remoto; não promete compatibilidade automática com qualquer API de modelo.

- [ ] Escrever `TestExplanationRequiresExplicitSend`: buscar/navegar/preview não faz request externo; gerar sem consentimento é recusado. `TestUnknownEvidenceRejected`: IDs inexistentes na resposta geram diagnóstico, sem referência inventada.
- [ ] Escrever UI test: antes do envio, mostrar destino e dados do pacote; após falha de provedor, grafo/busca continuam utilizáveis. Credenciais só no core e fora dos logs; cancelamento não inicia retry sem política explícita.
- [ ] Executar `go -C core test ./internal/application ./internal/adapters/explainer -run 'TestExplanation|TestUnknownEvidence'` e `npm test --workspace apps/extension -- ExplanationPanel.test.tsx`; confirmar falha.
- [ ] Implementar provider HTTP com timeout/cancelamento, esquema de resposta e pacote delimitado como dados; não executar instruções contidas no código analisado. Validar referências e distinguir afirmação apoiada de hipótese; não anunciar que citação existente garante correção semântica do texto.
- [ ] Reexecutar testes com servidor fake, registrar contrato/configuração do provider e verificação de schema; commit `feat: explain investigations with explicit evidence-backed AI requests`.

## Cobertura e conclusão

| Requisito / critério da spec | Tarefas |
| --- | --- |
| 1. Indexação com relações e evidências | 1, 2, 11, 12, 14 |
| 2. Zero a muitos efeitos | 1, 7, 8, 10, 16 |
| 3. Correlação sem coincidência temporal | 4, 6, 7, 8, 10, 16 |
| 4. Separação estático/observado | 1, 9, 16 |
| 5. JSF, JS intermediário, remoteCommand e endpoints | 7, 8, 12, 15, 16 |
| 6. Campos, tabelas e JRXML | 11, 13 |
| 7. Impacto com evidências e limites | 13 |
| 8. Lacunas, perdas e capturas incompletas | 4, 6, 9, 16 |
| 9. IntelliJ e fallback | 3, 9, 19 |
| 10. Correspondência de revisão | 3, 15, 17, 19 |
| 11. Índice incremental sem relações obsoletas | 2, 12 |
| 12. Combinações antiga/recente e suporte publicado | 10, 15, 17, 18 |
| 13. Windows 10 x64 sem Go | 5, 18, 19 |
| 14. Sanitização e limites em diagnósticos | 4, 6, 18, 19 |
| 15. Produto utilizável sem IA | 9, 13, 20 |
| Hexagonal/modular e contratos extensíveis | 1, 2, 5, 11–15, 20 |
| CI/release e funcionamento real distintos | 18, 19 |
| Ganho de tempo comprovável | 19 |

Concluir uma tarefa exige validar sua interface, testes e diff. Concluir um marco exige seus cenários completos. Publicação depende dos gates; compatibilidade do sistema privado e Windows 10 dependem dos resultados externos. Se essa máquina não estiver acessível ao executor, concluir os artefatos executáveis e registrar precisamente os cenários aguardando o usuário.

## Referências técnicas verificadas no planejamento

- [Native Messaging — Chrome](https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging): registro do host no Windows, framing e limite de mensagem. O plano escolhe limite interno menor e paginação.
- [Content scripts — WXT](https://wxt.dev/guide/essentials/content-scripts.html): distinção entre contextos de execução da extensão e da página.
- [WildFly 10 Final](https://www.wildfly.org/news/2016/01/30/WildFly-10-Final-is-now-available/): referência da família Java EE antiga.
- [WildFly 35](https://www.wildfly.org/news/2025/01/09/WildFly-35-is-released/): referência Jakarta EE 10 com Java 17/21.
- [PrimeFaces 15.0.0](https://github.com/primefaces/primefaces/releases/tag/15.0.0): versão escolhida do exemplo recente; sua integração integral será verificada em fixture.
- [MySQL Connector/J](https://dev.mysql.com/downloads/connector/j/8.4.html) e [guia 5.1](https://downloads.mysql.com/docs/connector-j-5.1-en.pdf): referências para seleção dos drivers dos exemplos.
- [JavaParser](https://github.com/javaparser/javaparser) e [Byte Buddy](https://bytebuddy.net/): ferramentas candidatas; versão concreta fixada e compatibilidade Java 8/21 verificada na introdução de cada módulo.
- [IntelliJ — abertura pela linha de comando](https://www.jetbrains.com/help/idea/opening-files-from-command-line.html): launcher com localização, usado em vez de assumir uma API HTTP interna.
- [CodeQL CLI](https://docs.github.com/en/code-security/concepts/code-scanning/codeql/codeql-cli) e [database create](https://docs.github.com/en/code-security/reference/code-scanning/codeql/codeql-cli-manual/database-create): requisitos e condições de uso; extração é configurada por backend e capacidades.

## Revisão do plano

Revisão feita contra as 12 seções e os 15 critérios da spec: cobertura mapeada acima; DTOs e assinaturas compartilhados identificados; cinco classes de risco possuem tarefas/testes responsáveis. Comandos são instruções futuras, não evidência de execução. Sem implementação iniciada.

O próximo passo é a revisão deste plano pelo usuário e a escolha de execução nativa ou orientada a subagentes. Se houver subagentes, respeitar exclusivamente os modelos/esforços autorizados nas instruções do usuário; não delegar usando o modelo do pai por conveniência.
