# LegacyLens: captura pelo menu de contexto

## Objetivo

Substituir o painel HTML atualmente injetado na aplicação por um submenu nativo da extensão no menu de contexto do Chrome/Edge. O submenu reúne a seleção do projeto e as ações de captura. A página `investigation.html` continua responsável por cadastrar projetos, indexar código e pesquisar/investigar resultados.

O projeto também deve distinguir a saúde do core HTTP da disponibilidade do host nativo. `legacylens.exe status` verifica somente o core; a extensão usa `chrome.runtime.connectNative` e depende do manifesto do host registrado no perfil do navegador.

## Abordagens consideradas

1. **Submenu nativo com lista de projetos e ações de captura (selecionada).** Projetos aparecem como opções de rádio. Iniciar/parar captura e abrir o gerenciamento ficam no mesmo submenu. Remove o painel injetado e mantém as operações próximas da página onde se captura.
2. **Último projeto selecionado.** O menu teria menos itens e dependeria de um projeto escolhido na página de gerenciamento. Isso não atende à seleção direta de projetos no menu solicitada.
3. **Manter o painel injetado e adicionar apenas o menu como atalho.** Duplicaria os controles e manteria o popup que motivou a mudança.

## Interface

O item pai `LegacyLens` aparece para páginas HTTP e HTTPS. Seus itens diretos são:

- cada projeto registrado, como item de rádio; a seleção é específica da aba ativa;
- `Iniciar captura`, habilitado quando há um projeto selecionado e não há captura ativa naquela aba;
- `Parar captura`, habilitado enquanto a aba tem uma captura ativa;
- `Gerenciar projetos`, que abre a página da extensão.

Se não houver projetos, o submenu oferece a ação para abrir o gerenciamento. Se o host nativo não responder, exibe um item desabilitado com o estado `Host nativo desconectado`; uma falha de conexão não deve parecer uma lista vazia. Os menus nativos não aceitam campos de formulário, então o registro de nome e pasta permanece em `investigation.html`.

O clique no ícone da extensão abre a página de gerenciamento/investigação, sem inserir interface no DOM da aplicação. Capturar ainda requer scripts de conteúdo para observar interações e PrimeFaces; esses scripts não criam popup ou controles visuais de LegacyLens na página.

## Fluxo e estado

1. Ao exibir o menu de contexto, o background consulta `project.list` pelo host nativo, reconstrói os itens de rádio e atualiza os estados habilitados para a aba atual.
2. Selecionar um projeto atualiza a seleção associada à aba na sessão do navegador.
3. `Iniciar captura` pede a permissão opcional do host da página diretamente no gesto do clique. Depois da permissão, o background injeta os scripts de captura na aba, inicia a sessão com `CaptureController` e envia ao content script o projeto/sessão para armar a seleção do elemento.
4. `Parar captura` encerra a sessão, instrui o content script a remover o estado de captura e abre `investigation.html` para o projeto e trace capturados.
5. `Gerenciar projetos` abre `investigation.html` sem parâmetro de captura. Depois de cadastrar um projeto, a próxima abertura do menu consulta a lista atualizada.

Os scripts de conteúdo e de mundo principal devem ser idempotentes para que a injeção explícita na aba atual não duplique listeners quando o Chrome já os tiver carregado após a concessão de permissão.

## Host nativo e diagnóstico

O guia Windows explica que core saudável não significa host nativo conectado. A instalação do host exige o registro do manifesto `io.legacylens.host` para o ID da extensão atualmente carregada. Quando a política local do PowerShell bloquear o script não assinado, o guia orienta a usar `Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass` apenas na sessão atual e executar `install-native-host.ps1` com a pasta extraída e o ID correto. O registro continua restrito ao usuário atual.

Falha ao consultar projetos deve aparecer como host desconectado no menu e como erro explicativo na página de gerenciamento. O fluxo não deve abrir painel injetado nem iniciar captura silenciosamente sem um projeto válido.

## Arquivos e responsabilidades

- `apps/extension/entrypoints/background.ts`: menu pai/filhos, consulta e atualização de projetos, seleção por aba, início/parada da captura e navegação para gerenciamento/investigação.
- `apps/extension/entrypoints/content.ts`: remove a interface HTML atual; recebe comandos de ciclo de captura e mantém o encaminhamento de eventos do navegador.
- `apps/extension/entrypoints/page.content.ts`: instrumenta eventos do PrimeFaces, Ajax e rede necessários para associar a interação observada à captura, sem renderizar interface.
- `apps/extension/wxt.config.ts`: declara `contextMenus`, `scripting`, `tabs`, armazenamento de sessão e permissões opcionais de host.
- `apps/extension/entrypoints/investigation/main.tsx`: permanece responsável por registrar e selecionar projetos para indexação e investigação.
- `docs/user/windows-installation.md` e `README.md`: instruções de operação, host nativo e diagnóstico.

## Verificação e critérios de aceite

- O build de Chrome MV3 inclui a permissão `contextMenus` e os dois scripts de captura.
- Recarregar a extensão cria apenas um submenu `LegacyLens`, sem entradas duplicadas.
- Com o host instalado e projetos registrados, o menu mostra projetos como opções de rádio e seleciona um projeto por aba.
- `Iniciar captura` solicita permissão no gesto, inicia a captura da aba e não cria um painel HTML; `Parar captura` encerra e abre a investigação correspondente.
- `Gerenciar projetos` abre a página existente de registro/indexação.
- Com o core em execução e o host não registrado, o menu mostra `Host nativo desconectado`; o guia explica a instalação e a diferença entre os dois processos.
- O clique no ícone abre a página da extensão, sem injetar popup.

## Diagnóstico de compatibilidade do menu

Após a instalação da versão com submenu, foi observado no navegador: `Service worker registration failed. Status code: 15` e `Uncaught TypeError: Cannot read properties of undefined (reading 'addListener')` no background; o submenu não aparecia. A inicialização registrava `contextMenus.onShown.addListener` sem verificar se o evento dinâmico estava disponível. Como o erro interrompe a execução do background antes do registro de `contextMenus.onClicked`, essa é a causa provável do menu ausente; o número de linha do bundle minificado, sozinho, não identifica o membro ausente.

A implementação agora trata `contextMenus.onShown` e `contextMenus.refresh()` como capacidades opcionais. Quando o evento não existe, registra um aviso e mantém a criação do menu por `onInstalled`/`onStartup`; quando `refresh()` não existe, mantém o menu criado sem a atualização dinâmica. O browser não precisa ser reiniciado para atualizar uma extensão: recarregue-a em `chrome://extensions` (ou `edge://extensions`) e recarregue a aba da aplicação. Se o erro persistir, registre a versão do navegador, o log completo do service worker e a disponibilidade de `chrome.contextMenus.onShown` para localizar outra API ausente.

Uma segunda falha no fluxo também foi identificada: a renderização consultava `project.list` antes de criar o item pai. Native Messaging pode levar até 30 segundos para falhar, então um host ausente ou sem resposta podia atrasar a criação de todo o submenu. O background agora cria primeiro `LegacyLens` com o estado desabilitado `Consultando host nativo…`, atualiza o menu, e só então consulta projetos para completar as ações. Assim a presença do submenu não depende da conexão com o host.

O aviso `Permissions policy violation: unload is not allowed in this document` e a referência a `content-scripts/page.js` foram vistos no contexto da página. O código da extensão não registra listener `unload`; portanto esses avisos são tratados como uma linha de investigação separada e não como causa confirmada do registro do submenu. Critério de aceite adicional: após recarregar a extensão, o worker deve permanecer ativo sem erro de `addListener` e o item pai `LegacyLens` deve aparecer no menu de contexto de uma página HTTP/HTTPS.
