# LegacyLens — desenho do produto

Data: 30/09/2026

Estado: desenho consolidado para revisão da spec escrita. A implementação depende da aprovação desta spec e do plano subsequente.

## 1. Objetivo e contexto

LegacyLens é uma ferramenta local para investigar sistemas legados, rastrear fluxos entre interface, código e banco de dados e avaliar o impacto potencial de alterações. O resultado deve reduzir o tempo necessário para localizar a implementação de uma regra e entender suas dependências.

O público inicial é o desenvolvedor que mantém aplicações Java com JSF/PrimeFaces. O produto deve admitir novas versões e linguagens por meio de adaptadores, sem vincular o domínio central a Java ou a uma biblioteca de análise.

A primeira validação real ocorrerá em outra máquina: Windows 10 x64, com código-fonte, navegador e JBoss/WildFly executando localmente. A stack informada é Java 8, JSF 2, PrimeFaces 5 e MySQL 5. É possível reiniciar o servidor com `-javaagent`. Não há Go instalado nessa máquina: os componentes serão distribuídos compilados.

A máquina de desenvolvimento não executa esse sistema real. Exemplos controlados e testes automatizados serão usados nela e no CI; a validação no sistema real será registrada separadamente.

A tarefa TickTick `6aa73c378f087a6320dcd184` é uma fonte de requisitos somente para leitura. Este trabalho não altera o TickTick.

## 2. Escopo completo e estratégia de entrega

O escopo inclui:

- Indexar repositórios e extrair chamadas e dependências.
- Buscar classes, métodos, campos, tabelas, endpoints e relatórios.
- Explorar relações de XHTML, JavaScript, EL, Java, SQL e JRXML.
- Investigar onde alterar uma regra e quais telas, regras e relatórios podem ser afetados.
- Visualizar fluxos, dependências e evidências.
- Capturar a próxima interação ou uma interação com um elemento selecionado.
- Correlacionar clique, JavaScript, HTTP e execução Java por `traceId`.
- Representar múltiplos caminhos e efeitos, incluindo chamadas dinâmicas.
- Abrir arquivo e linha no IntelliJ quando a localização for conhecida.
- Suportar a stack inicial e versões recentes mediante uma matriz explícita de compatibilidade.
- Acrescentar IA posteriormente, como interpretação dos dados estruturados com referências às evidências.

As entregas serão organizadas por fluxos completos. Cada etapa conecta os componentes necessários a um cenário verificável. O primeiro cenário é uma interação PrimeFaces correlacionada com execução Java e SQL e vinculada ao código-fonte. As etapas posteriores ampliam a cobertura até completar o escopo; não substituem o escopo completo por esse primeiro cenário.

## 3. Princípios do modelo

Um botão não corresponde necessariamente a um método Java. O modelo é um grafo de eventos e efeitos com cardinalidade zero a muitos.

O produto mantém dois conjuntos correlacionados:

1. Grafo estático: relações que a análise do código identifica como possíveis.
2. Trace de execução: eventos e relações observados em uma interação específica.

Uma relação estática não constitui evidência de execução. Uma ausência no trace não prova que um caminho seja impossível. A interface preserva essa distinção em todas as investigações.

O domínio comum compreende projeto, revisão de código, artefato, símbolo, localização, relação, evidência, interação, trace, evento e diagnóstico. Identificadores incluem o contexto do projeto e do índice; nomes de símbolos isolados não identificam uma entidade de forma suficiente.

Cada relação registra origem, destino, tipo, evidência e método de obtenção. As localizações apontam para arquivo e linha quando disponíveis. Relações sem destino resolvido preservam o trecho ou evento de origem e o diagnóstico da lacuna.

Eventos registram trace, sequência no produtor, instante, origem e referências de causalidade quando disponíveis. A visualização não transforma ordenação temporal em causalidade sem evidência. Tarefas assíncronas e múltiplas requisições podem gerar ramificações.

## 4. Arquitetura hexagonal e modular

O produto usará monorepo e módulos com responsabilidades e contratos definidos.

O domínio e os casos de uso do core não dependerão diretamente de WXT, JSF, PrimeFaces, CodeQL, JDBC, IntelliJ ou de um mecanismo específico de armazenamento. Tipos dessas tecnologias serão traduzidos nas bordas.

Os casos de uso incluem registrar projeto, indexar, atualizar índice, buscar, explorar relações, analisar impacto, iniciar e finalizar captura, ingerir eventos, consultar investigação e abrir uma localização.

As portas incluem análise de artefatos, armazenamento, recepção de eventos, comunicação com o navegador, identificação da aplicação executada e abertura no editor. Os adaptadores implementam essas portas.

Os adaptadores declaram tecnologias, versões e capacidades suportadas. O core consulta essas capacidades antes de oferecer uma operação e apresenta diagnósticos para capacidades indisponíveis. Uma linguagem adicional deve reutilizar o modelo comum e pode introduzir tipos de relação identificados sem exigir dependência do domínio na biblioteca de parsing.

Na primeira implementação, adaptadores são módulos versionados no monorepo. Carregamento de plugins externos não é requisito inicial.

### Componentes

| Componente | Responsabilidade | Interface principal |
| --- | --- | --- |
| Extensão TypeScript/WXT | Seleção de elementos, captura de interações e efeitos, integração JSF/PrimeFaces e interface de investigação | Protocolo de mensagens com o host nativo |
| Core Go | Projetos, casos de uso, coordenação da análise, grafos, traces, buscas, impacto e diagnósticos | Portas do domínio e protocolo local |
| Host de Native Messaging | Integrar a extensão ao core e gerenciar a conexão | Native Messaging e conexão local com o core |
| Adaptadores estáticos | Analisar XHTML/EL, JavaScript, Java, SQL e JRXML | Entrada de artefatos e saída de relações/evidências |
| Java Agent | Observar HTTP, execução Java e operações JDBC no servidor | Ingestão de eventos no core |
| Contratos e exemplos | Definir schemas, compatibilidade de protocolo e cenários de validação | Contratos versionados e aplicações de exemplo |

O host nativo é distribuído junto ao core. A extensão comunica-se com ele via Native Messaging. O agente comunica-se com o core por conexão local autenticada. O primeiro ambiente opera inteiramente na mesma máquina.

O core concentra a lógica de investigação; a extensão concentra DOM, interação, hooks do navegador e apresentação. O agente concentra instrumentação e emissão de eventos, sem implementar as regras de investigação.

## 5. Investigação no produto

A extensão oferece três entradas: selecionar um elemento, capturar a próxima interação ou buscar um símbolo no projeto.

### Investigação de uma interação

1. Selecionar um projeto indexado e verificar o estado dos componentes.
2. Ativar “Trace this element” ou “Trace next interaction”.
3. Registrar a interação e seus efeitos no navegador.
4. Correlacionar requisições com eventos do servidor quando houver evidência suficiente.
5. Apresentar caminhos observados, ramificações, múltiplas requisições, falhas e lacunas.
6. Acrescentar relações estáticas sob escolha do usuário.
7. Consultar evidências e abrir localizações no IntelliJ.

A captura suporta cadeias como `p:commandButton → JS → JS → p:remoteCommand → Faces Ajax → actionListener/action → métodos Java`. Também suporta `JS → fetch/XHR → Servlet/Filter/endpoint` e caminhos sem chamada ao servidor.

O identificador da interação agrupa seus efeitos. Propagação por `traceId` e relações de causalidade devem ser verificadas por cenário. O produto não deve tratar toda atividade de rede ou de servidor durante a janela de captura como parte da interação automaticamente.

### Busca e impacto

A busca aceita classe, método, campo, tabela, endpoint e relatório. Cada resultado oferece navegação de dependências, evidências e traces relacionados.

A análise de impacto percorre relações relevantes e apresenta os caminhos que sustentam os resultados. Ela identifica impacto potencial, incluindo incertezas de análise e trechos não cobertos. A análise de JRXML relaciona relatórios, expressões, consultas, campos e dependências extraíveis.

## 6. Indexação e evidências

Cada projeto permite configurar diretórios incluídos e excluídos. O processamento inicial cria o índice; atualizações reprocessam arquivos alterados e relações afetadas, removendo evidências obsoletas dos arquivos modificados ou excluídos.

Cada índice registra revisão do código, versão dos adaptadores e diagnósticos. O armazenamento deve permitir rastrear uma relação até sua evidência e até a versão do arquivo analisado.

CodeQL é um backend de análise estática. Requisitos de execução, extração/build e distribuição/licenciamento devem ser verificados no plano antes de definir o pacote distribuído. A execução na máquina de validação não deve pressupor uma instalação de Go. A ausência de um backend deve gerar diagnóstico de capacidades indisponíveis.

Expressões EL, reflexão e SQL dinâmico que não puderem ser resolvidos preservam as evidências disponíveis. Traces podem acrescentar resoluções observadas, mantendo o contexto daquela execução.

O índice e a aplicação executada têm identificações próprias. Correspondência de código deve ser confirmada por evidência disponível da versão implantada. Sem confirmação, a interface marca as localizações como correspondência não confirmada.

## 7. Compatibilidade

O agente inicial deve executar em Java 8. Suporte a versões recentes deve contemplar a transição de `javax` para `jakarta`, alterações de JSF/PrimeFaces e diferenças de servidor e driver JDBC por adaptadores.

| Estado | Critério |
| --- | --- |
| Validado | A combinação identificada passou pelos cenários automatizados definidos para suas capacidades |
| Experimental | Há suporte implementado, mas a validação da combinação está incompleta |
| Não suportado | A combinação ou capacidade não tem suporte implementado; a interface informa a limitação |

O plano de implementação deve selecionar versões exatas para duas combinações de referência: a stack inicial e uma combinação recente. A seleção recente requer verificação das compatibilidades oficiais entre seus componentes. Não se anuncia suporte genérico a todas as versões.

As versões menores exatas de JSF 2, PrimeFaces 5, MySQL 5 e JBoss/WildFly do sistema real serão coletadas na preparação da validação. Até essa coleta e execução dos cenários, a compatibilidade desse sistema não será apresentada como comprovada.

## 8. Falhas, limites e coleta

| Situação | Comportamento |
| --- | --- |
| Analisador falha | Preservar resultados válidos e identificar o índice como parcial, com diagnóstico por arquivo/capacidade |
| Agente desconectado | Manter análise estática e busca; informar ausência de eventos Java na captura |
| Captura interrompida ou limite atingido | Preservar eventos recebidos e marcar o trace como incompleto |
| Aplicação e código sem correspondência confirmada | Identificar a limitação junto às localizações e evidências |
| IntelliJ indisponível | Disponibilizar arquivo e linha para consulta manual |
| Adaptador incompatível | Informar capacidades indisponíveis e o estado de suporte |

Capturas e filas terão limites configuráveis. Limites atingidos e descartes devem produzir diagnósticos; perdas não podem ser silenciosas. A instrumentação deve evitar bloquear requisições da aplicação quando o core estiver indisponível ou sobrecarregado.

A coleta padrão registra metadados técnicos necessários à investigação. Cookies, tokens, corpos HTTP e valores de parâmetros SQL ficam fora da coleta padrão. Diagnósticos e exportações seguem essa regra. Consultas SQL podem conter literais sensíveis; a captura deve aplicar sanitização e informar quando isso limitar a evidência.

A aplicação permanece local por padrão. Conexões locais devem restringir acesso aos componentes autorizados. O envio de evidências a um provedor de IA será uma operação explícita na etapa futura de IA, com definição dos dados enviados.

## 9. Testes e critérios de aceitação

O CI deve verificar contratos de adaptadores, exemplos com relações esperadas e cenários completos. A quantidade de testes é definida pela cobertura dos comportamentos, sem depender do sistema privado do usuário.

Critérios de aceitação:

1. Indexar os artefatos dos exemplos e consultar símbolos e relações com evidências.
2. Representar zero a muitos efeitos por interação, incluindo múltiplas requisições.
3. Correlacionar clique, JavaScript, HTTP, Java e SQL nos cenários suportados sem associar atividade concorrente por mera proximidade temporal.
4. Diferenciar relações estáticas e eventos observados na apresentação e nos contratos.
5. Explorar cadeias JSF/PrimeFaces, chamadas JavaScript intermediárias, `remoteCommand` e endpoints.
6. Relacionar campos, tabelas e JRXML nos exemplos definidos.
7. Apresentar impacto potencial acompanhado dos caminhos e das limitações que sustentam a resposta.
8. Mostrar lacunas de resolução, desconexões, perdas e capturas incompletas.
9. Abrir localizações conhecidas no IntelliJ e oferecer consulta manual quando necessário.
10. Identificar correspondência não confirmada entre código e aplicação.
11. Atualizar o índice sem manter relações obsoletas de arquivos alterados ou excluídos.
12. Executar as combinações antiga e recente escolhidas e publicar seus estados de compatibilidade.
13. Instalar e executar os componentes distribuídos no Windows 10 x64 sem instalar Go.
14. Aplicar os limites de coleta e sanitização também aos diagnósticos.
15. Manter busca, navegação e investigação utilizáveis sem IA.

A validação real registra versão dos componentes, stack exata, identificação do código/aplicação, cenário executado, resultado e limitações. Uma release com CI aprovado não equivale à confirmação de funcionamento no sistema real.

O sucesso do produto será avaliado em investigações de manutenção: registrar tempo e resultado de uma investigação manual de referência e comparar com a investigação usando LegacyLens, verificando se a ferramenta identifica os pontos de implementação com evidências úteis. O ganho será medido, não presumido.

## 10. CI e distribuição

GitHub Actions executará as verificações obrigatórias e produzirá os artefatos versionados. A publicação no GitHub Releases será condicionada ao sucesso dessas etapas para a mesma revisão do código.

A distribuição inicial inclui executável Go para Windows x64, host/registro de Native Messaging, extensão, Java Agent e instruções de configuração e remoção. Os componentes são versionados em conjunto e identificam incompatibilidade de protocolo.

O plano definirá empacotamento e gatilho de publicação. A distribuição de backends externos, especialmente CodeQL, depende da verificação de requisitos e licenças; executáveis externos não serão presumidos como redistribuíveis.

As instruções devem cobrir seleção do projeto, instalação da extensão, registro do host nativo, conexão do core e configuração de `-javaagent` no JBoss/WildFly. A máquina de validação não compila o core.

## 11. Sequência de entregas

1. Fundação modular e primeiro fluxo completo: projeto, contratos, índice mínimo, captura PrimeFaces, correlação HTTP/Java/SQL, apresentação de evidências e distribuição executável.
2. Ampliação da análise e investigação: XHTML/EL, JavaScript, Java, SQL e JRXML; busca, navegação, impacto e atualização incremental.
3. Cobertura de integrações: cadeias JavaScript, `remoteCommand`, endpoints, ramificações, chamadas dinâmicas e resolução observada.
4. Compatibilidade e robustez: matriz antiga/recente, falhas parciais, limites, versões de protocolo e validação no legado real.
5. IA: explicações sobre dados estruturados, com referências e regras explícitas de envio de dados.

Os contratos, diagnósticos, limites e mecanismos de extensão começam na fundação e evoluem em cada etapa. A ordem não adia sua definição para o fim.

## 12. Decisões e limites do desenho

Estão aprovados em conversa: escopo integral da spec original, entregas por fluxos completos, divisão dos componentes, experiência de investigação, indexação, estados de compatibilidade, tratamento de falhas, arquitetura hexagonal/modular e estratégia de testes e releases.

Este documento consolida essas decisões para revisão escrita. Detalhes executáveis — bibliotecas, schemas concretos, versões exatas, armazenamento, hooks de instrumentação, navegador inicial e empacotamento — serão resolvidos no plano com evidências de compatibilidade. A seleção deverá cumprir os comportamentos e critérios deste documento.

A próxima etapa, após aprovação da spec escrita, é criar o plano de implementação com a skill `superpowers:writing-plans` e apresentar a escolha do método de execução. Nenhum código de produto foi autorizado por esta consolidação.
