# Explicações com evidências

LegacyLens pode preparar uma explicação de uma investigação e enviá-la a um endpoint HTTP configurado pelo usuário. A busca, o grafo e a pré-visualização permanecem locais. Nenhum provedor é chamado ao navegar ou ao gerar o preview.

## Configurar o provedor

Configure o processo do core antes de iniciá-lo:

```powershell
$env:LEGACYLENS_EXPLAINER_URL = 'https://provider.example/v1/explanations'
$env:LEGACYLENS_EXPLAINER_TOKEN = '<credencial-do-provedor>'
```

O token é opcional para provedores locais. O endpoint deve ser HTTPS; HTTP é aceito somente em loopback (`localhost` ou IP de loopback). URLs com credenciais, query ou fragmento são recusadas. Configure um endpoint que implemente o contrato interno [explanation-v1](../../contracts/explanation-v1.schema.json); isso não garante compatibilidade automática com APIs de terceiros.

Mantenha a credencial somente no ambiente do core, com acesso restrito. A extensão não recebe o token. O core não registra o token nem o corpo da resposta. Para trocar o provedor, feche e reabra o core com a nova configuração.

## Pré-visualizar e enviar

1. Abra uma investigação e marque de 1 a 20 evidências.
2. Escreva uma pergunta e escolha **Pré-visualizar envio**. Essa operação consulta somente a investigação local.
3. Revise o destino e o JSON exato exibido. O pacote contém a pergunta, revisões conhecidas, estado de completude e, para cada item selecionado, ID, tipo e caminho relativo/linha/coluna quando disponíveis.
4. O pacote não inclui nome ou caminho raiz do projeto, texto-fonte, eventos, metadados de eventos, SQL, logs, trace completo, variáveis de ambiente ou credenciais. A pergunta é texto digitado pelo usuário e deve ser revisada como parte do pacote.
5. Marque o consentimento e escolha **Enviar e gerar explicação**. O preview expira em cinco minutos e só pode ser usado uma vez. O envio é feito pelo core ao destino mostrado.
6. Revise cada afirmação e seus IDs de evidência. Afirmações apoiadas exigem pelo menos uma referência pertencente ao pacote; referência desconhecida invalida a resposta inteira. Hipóteses são rotuladas separadamente. Limitações e aviso de verificação permanecem visíveis.

Não há retry automático. Se a chamada falhar ou for cancelada, faça um novo preview e dê consentimento novamente antes de tentar outra vez. A falha do provedor não bloqueia a investigação, a busca nem o grafo. Uma citação válida confirma que o ID existe no pacote, não que a frase gerada esteja semanticamente correta.

## Contrato e limites

O core envia um POST JSON com `protocolVersion`, `requestId` e `data`; a resposta contém `claims` (`text`, `evidenceIds`, `confidence`) e `limitations`. O corpo de resposta é limitado a 256 KiB, a operação tem timeout e redirecionamentos são recusados para evitar encaminhar credenciais a outro host. O contrato versionado é a fonte normativa.

O conteúdo analisado é transmitido como dados delimitados e não é executado pelo LegacyLens. Antes do consentimento, qualquer texto na pergunta e o destino completo são apresentados ao usuário. Use apenas provedores e dados que você está autorizado a compartilhar.
