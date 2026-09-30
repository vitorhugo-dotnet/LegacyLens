# Abrir localizações no IntelliJ IDEA

O LegacyLens pode iniciar o launcher de linha de comando do IntelliJ IDEA. Configure a variável de ambiente `LEGACYLENS_INTELLIJ_LAUNCHER` com `idea`, `idea64.exe` ou o caminho completo do executável. No PowerShell, por exemplo:

```powershell
$env:LEGACYLENS_INTELLIJ_LAUNCHER = 'C:\Program Files\JetBrains\IntelliJ IDEA\bin\idea64.exe'
```

Um launcher passado diretamente ao construtor do adapter tem precedência sobre a variável de ambiente.

Ao abrir uma localização, o LegacyLens verifica que o arquivo regular existe dentro da raiz do projeto registrado. Em seguida executa o launcher com argumentos separados: `--line`, o número da linha e o caminho absoluto do arquivo. O LegacyLens não usa shell nem depende de uma API HTTP do IntelliJ.

Se o launcher não estiver configurado, a resposta contém o caminho e a linha para abertura manual. Se o processo do launcher falhar, a mesma informação acompanha o erro para servir de fallback. `Opened` indica que o processo do launcher terminou com sucesso; isso não confirma visualmente que a janela ou o arquivo apareceu no IDE.

## Revisões

A comparação de revisão retorna `confirmed` somente quando projeto, identidade e digest estão presentes e coincidem. Uma diferença em qualquer identidade conhecida resulta em `mismatch`; campos ausentes resultam em `unconfirmed`. Revisão não identificada pela implantação não é tratada como confirmada.
