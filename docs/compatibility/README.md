# Matriz de compatibilidade

[`matrix.json`](matrix.json) registra combinações de runtime realmente executadas, as versões detectadas e os recursos exercitados. A matriz é evidência por combinação; ela não implica suporte a versões intermediárias ou a outros servidores.

## Reexecutar no Windows

Os testes baixam distribuições fixadas em `fixtures/legacy/fixture-lock.json` e `fixtures/modern/fixture-lock.json`, verificam os checksums e iniciam servidores e bancos isolados dentro de `.fixture-cache`.

```powershell
npm test --workspace tests/e2e -- legacy-click.spec.ts
npm test --workspace tests/e2e -- modern-click.spec.ts
npm test --workspace tests/e2e -- redeploy.spec.ts
```

O fixture antigo cobre Java 8, Java EE (`javax`), WildFly 10, PrimeFaces 5, MySQL 5.7 e Connector/J 5.1. O fixture recente cobre Java 21, Jakarta EE, WildFly 35, PrimeFaces Jakarta 15, MySQL 8.4 e Connector/J 8.4.

## Como as versões são detectadas

- Java é lido de `java.runtime.version` dentro da aplicação implantada.
- WildFly vem de `:product-info` no CLI conectado ao servidor em execução.
- Faces e PrimeFaces vêm do runtime implantado; os logs de inicialização do servidor são mantidos como evidência da implementação carregada.
- MySQL é consultado com `SELECT VERSION()` na conexão usada pelo fixture.
- Connector/J é identificado com `DatabaseMetaData.getDriverVersion()` na mesma conexão.
- A revisão do WAR recente é filtrada no build e exibida pela aplicação. A E2E confirma a troca de revisão A para B e verifica a troca do classloader que produz os eventos.

`stop.ps1` encerra os processos associados ao estado do fixture e remove seus dados temporários. Execute-o se uma execução for interrompida antes da limpeza automática.

O baseline usa uma amostra de cada fixture. `captureToDaoEvidenceMs` mede desde o início da captura até o backend entregar o evento `OrderDao#insert`; evento e relação contam o conteúdo daquela investigação. Esses números servem como ponto de comparação para execuções futuras, não como meta de desempenho.

## Limites

As execuções registradas nesta matriz ocorreram em Windows 11 Home x64. O produto também tem como alvo Windows 10 x64, mas essa validação deve ser registrada separadamente na Task 19. Não anuncie versões como compatíveis até que a respectiva combinação e seus fluxos tenham sido executados e registrados.
