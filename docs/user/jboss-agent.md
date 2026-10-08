# Agente em JBoss/WildFly no Windows

Use somente um pacote e uma aplicação autorizados. O agente precisa ser iniciado pela JVM que hospeda o deployment. Adicionar a opção a outro terminal ou à JVM do IntelliJ não instrumenta o servidor.

## Arquivo de configuração

Guarde o arquivo fora do diretório do produto, com ACL restrita ao usuário/conta de serviço que executa o servidor. Não o versione, anexe a tickets nem inclua em diagnóstico compartilhado.

```properties
endpoint=http://127.0.0.1:43127/v1/events
token=<token-do-core-local>
projectId=<id-do-projeto>
producerId=<id-unico-do-servidor>
revision=<revisao-realmente-implantada>
packages=<prefixo.java.autorizado>,<outro.prefixo.autorizado>
```

Use o endpoint local e o token configurados pelo próprio core. `packages` deve listar os pacotes da aplicação a instrumentar; evite instrumentar bibliotecas de terceiros. `revision` deve identificar o artefato implantado, não apenas o checkout local.

## Inicialização por `standalone.conf.bat`

Faça cópia de segurança local do arquivo de inicialização e altere o arquivo efetivamente usado pelo serviço/atalho. Em instalação standalone padrão, `standalone.bat` lê `bin\standalone.conf.bat` quando presente. Adicione à definição existente de `JAVA_OPTS`, preservando as opções atuais:

```bat
set "JAVA_OPTS=%JAVA_OPTS% -javaagent:C:/LegacyLens/java/legacylens-agent.jar=config=C:/Users/<usuario>/AppData/Local/LegacyLens/agent.properties"
```

Use caminhos absolutos. Barras `/` evitam a necessidade de escapar `\` no argumento da JVM. Se o serviço usa wrapper, domain mode ou configuração própria, aplique a opção no mecanismo que monta os argumentos da JVM do processo gerenciado; não presuma que `standalone.conf.bat` está sendo lido.

Reinicie o processo de servidor de forma controlada. Confira o comando efetivo/diagnóstico local para confirmar que a JVM recebeu `-javaagent:...=config=...`; nunca publique essa linha se ela contiver caminhos ou opções sensíveis. Verifique o log de inicialização e o estado do deployment. Um aviso `LegacyLens agent configuration failed` deve ser tratado como falha de configuração, não como captura ativa.

## Rollback

Remova somente a opção acrescentada, restaure as opções anteriores e reinicie o mesmo processo de servidor. Confirme que o deployment voltou ao estado esperado. Não pare outros serviços nem apague evidências do host/usuário que não pertençam ao pacote instalado.

## Segurança e compatibilidade

O agente envia eventos ao core local com bearer token. Restrinja o arquivo de configuração, use token dedicado, mantenha o endpoint em loopback e não exponha o token em logs ou screenshots. A compatibilidade deve ser verificada para a combinação exata de Windows, Java, JBoss/WildFly, JSF, PrimeFaces, MySQL e driver. A fixture e o runner hospedado não comprovam a instalação real.
