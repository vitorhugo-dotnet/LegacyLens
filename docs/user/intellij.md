# Abrir localizações no IntelliJ IDEA

O LegacyLens pode iniciar o launcher de linha de comando do IntelliJ IDEA. Configure a variável de ambiente `LEGACYLENS_INTELLIJ_LAUNCHER` com `idea`, `idea64.exe` ou o caminho completo do executável. No PowerShell, por exemplo:

```powershell
$env:LEGACYLENS_INTELLIJ_LAUNCHER = 'C:\Program Files\JetBrains\IntelliJ IDEA\bin\idea64.exe'
```

Um launcher passado diretamente ao construtor do adapter tem precedência sobre a variável de ambiente.

Ao abrir uma localização, o LegacyLens verifica que o arquivo regular existe dentro da raiz do projeto registrado. Antes de executar o launcher, reabre o caminho sob a raiz e confere identidade do arquivo e SHA-256; o descritor original e a raiz permanecem abertos enquanto o launcher executa. Alterações detectadas ou um redirecionamento para fora da raiz retornam erro com arquivo e linha para fallback. A leitura do hash é feita em streaming e respeita cancelamento.

Em seguida executa o launcher com argumentos separados: `--line`, o número da linha e o caminho absoluto do arquivo. O LegacyLens não usa shell nem depende de uma API HTTP do IntelliJ.

Essa verificação reduz e detecta substituições ocorridas antes da chamada, mas não torna atômica a resolução posterior do caminho pelo processo externo. O projeto deve estar em uma árvore confiável, sem mutação concorrente durante a abertura. Não use este fluxo para abrir código em uma raiz que outro processo não confiável possa alterar ao mesmo tempo.

Se o launcher não estiver configurado, a resposta contém o caminho e a linha para abertura manual. Se o processo do launcher falhar, a mesma informação acompanha o erro para servir de fallback. `Opened` indica que o processo do launcher terminou com sucesso; isso não confirma visualmente que a janela ou o arquivo apareceu no IDE.

## Revisões

A comparação de revisão retorna `confirmed` somente quando projeto, identidade e digest estão presentes e coincidem. Uma diferença em qualquer identidade conhecida resulta em `mismatch`; campos ausentes resultam em `unconfirmed`. Revisão não identificada pela implantação não é tratada como confirmada.
