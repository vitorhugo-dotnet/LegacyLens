# Instalação no Windows x64

O ZIP de release contém o core, o host de mensagens nativas, a extensão Chromium unpacked, o agente e worker Java, o manifesto de proveniência e as evidências das combinações testadas. Windows 10 x64 é alvo; a execução de validação do release ocorre em runner Windows hospedado e não substitui a validação específica no Windows 10 registrada na matriz.

## Instalar

1. Extraia `legacylens-windows-x64.zip` para uma pasta permanente, por exemplo `C:\Program Files\LegacyLens`.
2. Abra Chrome ou Edge em `chrome://extensions` ou `edge://extensions`, ative o modo de desenvolvedor e carregue a pasta `extension` do pacote.
3. Copie o ID exibido pela página de extensões. Execute PowerShell na pasta extraída:

   ```powershell
   .\scripts\install-native-host.ps1 -PackageDirectory 'C:\Program Files\LegacyLens' -ExtensionId 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
   ```

   O script grava o manifesto numa subpasta própria em `%LOCALAPPDATA%\LegacyLens\hosts` e registra somente o host `io.legacylens.host` no perfil atual do usuário e nas chaves de Chrome e Edge; não precisa de elevação mesmo que o pacote esteja em `Program Files`. Repetir o comando é seguro. Instalações lado a lado ficam isoladas, e remover a pasta antiga não desregistra a atual. O ID deve ser o da extensão carregada.

4. Inicie o core:

   ```powershell
   $env:LEGACYLENS_EXTENSION_ID = '<ID copiado da página de extensões>'
   .\legacylens.exe serve
   ```

   Use o mesmo ID passado em `-ExtensionId` no passo anterior. Sem essa variável, o core não autoriza a origem da extensão, mesmo que `status` reporte o serviço HTTP como saudável. A variável vale somente para essa janela do PowerShell.

5. Abra uma aplicação JSF compatível. Clique com o botão direito na página e abra **LegacyLens**. Selecione o projeto, reabra o submenu e escolha **Iniciar captura**. Autorize o acesso ao site quando solicitado, selecione o elemento JSF e execute a interação. Para finalizar, escolha **Parar captura**; a investigação da captura será aberta. **Gerenciar projetos** e o ícone da extensão abrem a página de registro, indexação e pesquisa.

## Diagnóstico do host nativo

O comando `legacylens.exe status` verifica somente a API HTTP do core. A extensão conversa com `legacylens-host.exe` pelo Native Messaging do navegador, que também exige o manifesto `io.legacylens.host` registrado no perfil atual e autorizado para o ID exato da extensão. Por isso, o core pode responder como saudável enquanto o submenu mostra **Host nativo desconectado**.

Confira estes pontos:

1. Deixe `legacylens.exe serve` em execução e confirme a resposta com `legacylens.exe status`.
2. Em `chrome://extensions` ou `edge://extensions`, confirme que a extensão está habilitada e copie novamente seu ID. O ID deve ser o mesmo usado em `-ExtensionId` e em `$env:LEGACYLENS_EXTENSION_ID`; recarregar uma extensão unpacked com outra identidade exige registrar o host e reiniciar `serve` com o novo ID.
3. Confirme que o pacote extraído ainda existe no caminho informado em `-PackageDirectory` e que contém `legacylens-host.exe`.
4. Execute novamente o instalador para o usuário atual. Se o PowerShell bloquear o script por política de execução, abra uma sessão PowerShell nessa pasta e rode:

   ```powershell
   Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass -Force
   .\scripts\install-native-host.ps1 -PackageDirectory 'C:\Program Files\LegacyLens' -ExtensionId '<ID exibido pelo navegador>'
   ```

   O escopo `Process` vale somente para essa janela do PowerShell; não altera a política permanente do usuário ou da máquina. O registro também é feito somente no usuário atual.

5. Recarregue a extensão na página de extensões e abra novamente o menu de contexto. Se o host continuar desconectado, confirme se o navegador e o host estão sendo executados no mesmo usuário do Windows e repita a instalação com o ID atualmente exibido.

Se o Chrome informar `Error when communicating with the native messaging host` depois de a mensagem `Specified native messaging host not found` ter desaparecido, confirme que `serve` foi iniciado na mesma sessão do PowerShell com `$env:LEGACYLENS_EXTENSION_ID` definido. Pare o processo `serve`, defina a variável e inicie-o novamente; em seguida, atualize `investigation.html`.

Não é necessário instalar Go ou Node. Java é necessário para executar o agente/analisador incluído. A análise estática básica continua disponível sem CodeQL CLI.

## Desinstalar

Com a mesma pasta do pacote e do mesmo usuário, execute:

```powershell
.\scripts\uninstall-native-host.ps1 -PackageDirectory 'C:\Program Files\LegacyLens'
```

O script remove apenas as chaves cujo valor aponta para o manifesto desse pacote e o próprio manifesto quando não houver registro restante. Ele não remove a pasta do produto nem outras chaves do registro.

## Compatibilidade

Consulte `docs/compatibility-matrix.json` e `release-manifest.json` para runtimes realmente testados. Os dados são referências de compatibilidade; versões intermediárias e outros servidores precisam de validação própria. A fixture moderna reproduzível e os checksums estão em `fixtures/modern`.
