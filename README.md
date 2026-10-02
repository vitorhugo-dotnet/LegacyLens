# LegacyLens

LegacyLens é uma ferramenta local em desenvolvimento para investigar aplicações legadas e relacionar interações capturadas no navegador com eventos observados no servidor. O foco inicial é manutenção de aplicações Java com JSF e PrimeFaces.

## O que existe hoje

- Core Go com CLI para iniciar a API local, registrar projetos, indexar XHTML e consultar capturas.
- Extensão Chromium Manifest V3 para selecionar elementos e capturar interações.
- Agente Java (`-javaagent`) para observar eventos de métodos e JDBC, compilado para Java 8.
- Host nativo Windows para comunicação local entre extensão e core.
- Contratos versionados para mensagens e eventos.

A análise Java estática e o worker Java estão implementados; suporte amplo a linguagens e a fixture moderna ainda não estão concluídos. O fluxo E2E da fixture antiga está falhando; CI executa esse cenário e deve permanecer vermelho até sua correção. Não há validação concluída contra o sistema privado nem em Windows 10.

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
mvn -f java/pom.xml -pl agent,analyzer -am verify
```

O E2E requer Chromium de teste e a fixture local Maven; sua falha atual é conhecida e não representa validação de uma instalação real. O empacotamento inicial é Windows x64 e inclui core, host nativo, extensão unpacked, agente e analyzer Java sombreado:

```powershell
./scripts/package-release.ps1
```

Esse pacote é experimental e não promete instalador, registro automático do host ou compatibilidade validada com uma máquina privada. A extensão deve ser carregada como unpacked e sua identidade precisa corresponder à configuração do host antes de uma futura instalação assistida.

## Estado e limitações

O workflow de CI executa contratos, core, extensão, agente, fixture antiga e E2E. A release por tag `v*` só publica quando o workflow reutilizável termina com sucesso para o mesmo SHA da tag; falha, cancelamento ou job ignorado bloqueiam publicação. Até o E2E passar, releases ficam bloqueadas.

O workflow instala o Chromium do Playwright, verifica o agente e o analyzer sombreado nos runtimes Java 8 e 21, e executa smokes do core, agente e analyzer empacotados. O smoke do core roda com Go e Node ausentes do `PATH`. Essas verificações não corrigem uma falha do E2E: pacote e publicação continuam condicionados ao cenário da fixture antiga e à fixture moderna com redeploy. O manifesto registra o analyzer como concluído e mantém a fixture moderna pendente; o workflow de release exige ambos os estados concluídos e os artefatos listados. Ainda faltam testes de instalação/remoção do host e validação do sistema real em Windows 10. CI verde não provará compatibilidade com o sistema privado.
