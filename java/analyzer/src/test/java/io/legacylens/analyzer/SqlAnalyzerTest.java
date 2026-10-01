package io.legacylens.analyzer;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Paths;
import static org.junit.jupiter.api.Assertions.*;

class SqlAnalyzerTest {
    private final ObjectMapper json = new ObjectMapper();

    @Test void joinAliasesAndBackticksExposeTableAndColumnDependencies() throws Exception {
        String source = new String(Files.readAllBytes(Paths.get("..", "..", "fixtures", "static", "sql", "queries.sql")), StandardCharsets.UTF_8);
        JsonNode result = new SqlAnalyzer().analyze(input(source), json);
        String symbols = result.path("symbols").toString();
        assertTrue(symbols.contains("orders"));
        assertTrue(symbols.contains("customers"));
        assertTrue(symbols.contains("customer_id"));
        assertTrue(result.path("relations").size() >= 2);
        long queries = 0;
        for (JsonNode symbol : result.path("symbols")) if (symbol.path("kind").asText().equals("query")) queries++;
        assertEquals(2, queries);
        assertEquals(result.path("relations").size(), result.path("evidence").size());
    }

    @Test void malformedSqlHasSanitizedDiagnostic() throws Exception {
        JsonNode result = new SqlAnalyzer().analyze(input("SELECT 'private-token' FROM WHERE"), json);
        assertFalse(result.path("diagnostics").isEmpty());
        assertFalse(result.toString().contains("private-token"));
    }

    private JsonNode input(String source) {
        com.fasterxml.jackson.databind.node.ObjectNode value = json.createObjectNode();
        value.put("projectId", "p"); value.put("revisionId", "r");
        value.putArray("artifacts").addObject().put("id", "a").put("path", "queries.sql").put("language", "sql");
        value.putObject("sources").put("queries.sql", source);
        return value;
    }
}
