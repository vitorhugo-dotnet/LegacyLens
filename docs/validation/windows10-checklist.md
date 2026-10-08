# Validação em Windows 10 x64

Execute este roteiro na máquina de validação, com navegador e aplicação autorizada disponíveis. A coleta é opt-in: nada é enviado à LegacyLens automaticamente.

## Preparação e proveniência

1. Anote o SHA ou versão exata do pacote e confirme o SHA-256 do ZIP antes de extrair.
2. Registre as versões efetivamente em execução: Windows/build, Java runtime, JBoss/WildFly e modo de inicialização, JSF, PrimeFaces, MySQL e driver JDBC. Use o log de inicialização/management CLI, metadados da implementação e `DatabaseMetaData`; não deduza versões de nomes de pastas ou configuração.
3. Colete apenas o estado sanitizado, se necessário:

   ```powershell
   .\scripts\collect-diagnostics.ps1 -OutputPath "$env:USERPROFILE\Desktop\legacylens-diagnostics.json" -Commit '<sha-do-pacote>' -Release '<versao>'
   ```

   O arquivo informa plataforma e cenários `not-run`. Ele não lê código-fonte, configurações, variáveis de ambiente, logs, traces ou credenciais. Inspecione e remova qualquer informação local adicional antes de compartilhar manualmente.

## Instalação e cenários

- [ ] Instalar no perfil atual; registrar versão do Chrome/Edge e ID explícito da extensão.
- [ ] Iniciar e parar o core, confirmar estado `status` e handshake da extensão.
- [ ] Instalar e remover o Native Messaging Host; verificar que apenas as chaves do perfil atual e do pacote testado foram alteradas.
- [ ] Configurar o agente no modo de inicialização realmente usado pelo JBoss/WildFly, reiniciar, conferir a mensagem de agente e confirmar que o deploy continua saudável.
- [ ] No IntelliJ, abrir uma localização com caminho e linha retornados pela investigação.
- [ ] Interagir sem gerar request de rede e confirmar que nenhum evento de servidor foi atribuído a essa interação.
- [ ] Executar um fluxo autorizado de ponta a ponta: interação JSF/PrimeFaces, request, método, serviço, DAO e escrita observável no banco de teste.
- [ ] Consultar impacto de uma tabela/campo e verificar cada aresta contra arquivo/linha e evidência apresentada.
- [ ] Se houver redeploy, comparar revisão e classloader; evidências anteriores não podem ser atribuídas à implantação nova.
- [ ] Repetir os cenários críticos após reiniciar o servidor e depois de desinstalar o host.

Atualize o resultado somente após executar cada cenário. Use `passed`, `failed` ou mantenha `not-run`, com evidência não sensível e versões exatas. Não inclua dump de trace, fonte, URL com parâmetros, token, arquivo de propriedades ou SQL literal. A matriz só pode ser atualizada com resultados reais.

## Diagnóstico e limites

Registre horário, ID do cenário, resultado e mensagem sanitizada. Para problemas, preserve localmente os logs originais e compartilhe trechos revisados manualmente. A coleta automática de LegacyLens não lê nem envia esses arquivos.

A execução em runner hospedado do GitHub e as fixtures não comprovam compatibilidade do Windows 10 nem de uma instalação privada. Declare cada combinação exata e cada cenário separadamente; não generalize para outras versões menores ou servidores.
