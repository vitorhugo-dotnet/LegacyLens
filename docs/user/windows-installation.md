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
   .\legacylens.exe serve
   ```

5. Abra uma aplicação JSF compatível, selecione uma interação e inicie a captura pela extensão.

Não é necessário instalar Go ou Node. Java é necessário para executar o agente/analisador incluído. A análise estática básica continua disponível sem CodeQL CLI.

## Desinstalar

Com a mesma pasta do pacote e do mesmo usuário, execute:

```powershell
.\scripts\uninstall-native-host.ps1 -PackageDirectory 'C:\Program Files\LegacyLens'
```

O script remove apenas as chaves cujo valor aponta para o manifesto desse pacote e o próprio manifesto quando não houver registro restante. Ele não remove a pasta do produto nem outras chaves do registro.

## Compatibilidade

Consulte `docs/compatibility-matrix.json` e `release-manifest.json` para runtimes realmente testados. Os dados são referências de compatibilidade; versões intermediárias e outros servidores precisam de validação própria. A fixture moderna reproduzível e os checksums estão em `fixtures/modern`.
