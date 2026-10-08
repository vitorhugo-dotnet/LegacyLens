# LegacyLens

LegacyLens é uma ferramenta local em desenvolvimento para investigar aplicações legadas e relacionar interações capturadas no navegador com eventos observados no servidor. O foco inicial é manutenção de aplicações Java com JSF e PrimeFaces.

## O que existe hoje

- Core Go com CLI para iniciar a API local, registrar projetos, indexar XHTML e consultar capturas.
- Extensão Chromium Manifest V3 para selecionar elementos e capturar interações.
- Agente Java (`-javaagent`) para observar eventos de métodos e JDBC, compilado para Java 8.
- Host nativo Windows para comunicação local entre extensão e core.
- Contratos versionados para mensagens e eventos.

A análise Java estática, o worker Java e as fixtures antiga e moderna estão implementados. Os fluxos E2E das duas fixtures passaram no CI do commit atual. Isso ainda não representa validação contra o sistema privado nem em Windows 10.

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

O E2E requer Chromium de teste e a fixture local Maven. O empacotamento inicial é Windows x64 e inclui core, host nativo, extensão unpacked, agente e analyzer Java sombreado:

```powershell
./scripts/package-release.ps1
```

Esse pacote é experimental e não promete instalador, registro automático do host ou compatibilidade validada com uma máquina privada. A extensão deve ser carregada como unpacked e sua identidade precisa corresponder à configuração do host antes de uma futura instalação assistida.

## Como usar

1. Baixe o `legacylens-windows-x64.zip` mais recente na aba [Releases](https://github.com/vitorhugo-dotnet/LegacyLens/releases) e extraia-o numa pasta permanente.
2. Carregue a pasta `extension` no Chrome ou Edge pela página de extensões, com o modo de desenvolvedor ativado.
3. Registre o host nativo para o ID da extensão e registre a pasta-fonte que deseja investigar. Os comandos completos estão em [Instalação no Windows](docs/user/windows-installation.md).

   ```powershell
   .\scripts\install-native-host.ps1 -PackageDirectory 'C:\Program Files\LegacyLens' -ExtensionId '<ID exibido pelo navegador>'
   .\legacylens.exe project register --root 'C:\src\MinhaAplicacao' --name 'MinhaAplicacao'
   .\legacylens.exe serve
   ```

4. Deixe o terminal com `serve` aberto. Abra a aplicação JSF no Chrome ou Edge, escolha o projeto registrado no controle da extensão sobre a página e inicie uma captura antes de realizar a interação. Ao parar a captura, a extensão abre a investigação; também é possível indexar o projeto e buscar código na página de investigação. Os dados do projeto são mantidos localmente. Em outro terminal, execute ` .\legacylens.exe status` para verificar se o core está respondendo.

### Publicação de releases

Um push para `main` publica automaticamente uma Release depois que os testes e a criação do pacote Windows terminarem com sucesso. O workflow cria uma tag no formato `DATA.PATCH`, usando a data de `America/Fortaleza` e o próximo número livre entre as Releases do mesmo dia: por exemplo, `20261008.1` e depois `20261008.2`. O número não depende das execuções do GitHub Actions. A Release recebe o ZIP e `SHA256SUMS.txt`; pull requests e pushes em outras branches não publicam.

## Estado e limitações

O workflow de CI executa contratos, core, extensão, agente, fixtures antiga e moderna e seus fluxos E2E. A publicação automática em `main` depende do sucesso de todos esses jobs e do empacotamento verificado. O fluxo separado acionado por tags `v*` também exige que o workflow reutilizável termine com sucesso para o mesmo SHA da tag; falha, cancelamento ou job ignorado bloqueiam essa publicação.

O workflow instala o Chromium do Playwright, verifica o agente e o analyzer sombreado nos runtimes Java 8 e 21, e executa smokes do core, agente e analyzer empacotados. O smoke do core roda com Go e Node ausentes do `PATH`. Pacote e publicação continuam condicionados aos E2E das fixtures antiga e moderna com redeploy. O manifesto registra o analyzer e a fixture moderna como concluídos e o workflow de release exige esses estados e os artefatos listados. Ainda faltam testes de instalação/remoção do host e validação do sistema real em Windows 10. CI verde não prova compatibilidade com o sistema privado.
