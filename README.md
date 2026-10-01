# LegacyLens

LegacyLens é uma ferramenta local em desenvolvimento para investigar aplicações legadas e relacionar interações capturadas no navegador com eventos observados no servidor. O foco inicial é manutenção de aplicações Java com JSF e PrimeFaces.

## O que existe hoje

- Core Go com CLI para iniciar a API local, registrar projetos, indexar XHTML e consultar capturas.
- Extensão Chromium Manifest V3 para selecionar elementos e capturar interações.
- Agente Java (`-javaagent`) para observar eventos de métodos e JDBC, compilado para Java 8.
- Host nativo Windows para comunicação local entre extensão e core.
- Contratos versionados para mensagens e eventos.

A análise Java estática, o worker Java, suporte amplo a linguagens e a fixture moderna ainda não estão implementados. O fluxo E2E da fixture antiga está falhando; CI executa esse cenário e deve permanecer vermelho até sua correção. Não há validação concluída contra o sistema privado nem em Windows 10.

## Arquitetura

O core mantém projetos e evidências localmente. A extensão observa a interação no navegador e conversa com o core pelo host nativo. O agente instrumenta o processo Java iniciado com `-javaagent` e envia eventos ao core. Os componentes usam contratos versionados; correlação entre navegador e servidor depende de evidências efetivamente observadas.

## Desenvolvimento local

Toolchains registradas: Go 1.26.2, Node.js 22.15.0 com npm 10.9.2, JDK 21 e Maven. O agente é compilado com alvo Java 8. Para preparar e verificar os componentes atuais:

```powershell
npm ci
npm test --workspace packages/contracts
npm run typecheck --workspace apps/extension
npm test --workspace apps/extension
go -C core test ./...
mvn -f java/pom.xml -pl agent -am verify
```

O E2E requer Chromium de teste e a fixture local Maven; sua falha atual é conhecida e não representa validação de uma instalação real. O empacotamento inicial é Windows x64 e inclui core, host nativo, extensão unpacked e agente:

```powershell
./scripts/package-release.ps1
```

Esse pacote é experimental e não promete instalador, registro automático do host ou compatibilidade validada com uma máquina privada. A extensão deve ser carregada como unpacked e sua identidade precisa corresponder à configuração do host antes de uma futura instalação assistida.

## Estado e limitações

O workflow de CI executa contratos, core, extensão, agente, fixture antiga e E2E. A release por tag `v*` só publica quando o workflow reutilizável termina com sucesso para o mesmo SHA da tag; falha, cancelamento ou job ignorado bloqueiam publicação. Até o E2E passar, releases ficam bloqueadas.

O worker de análise Java e a fixture moderna com redeploy continuam como extensões pendentes do Task18. Também faltam verificação automatizada do pacote sem toolchains, testes da instalação/remoção do host, validação completa em runtimes Java 8 e 21 e validação do sistema real em Windows 10. A existência de CI verde não provará compatibilidade com o sistema privado.
