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

    @Test void updateWritesAndLocationsFollowSourceLines() throws Exception {
        String source = "-- before\nSELECT\n  o.id\nFROM orders o\nJOIN customers c ON c.id = o.customer_id;\nUPDATE orders\nSET status = ?\nWHERE id = ?;";
        JsonNode result = new SqlAnalyzer().analyze(input(source), json);
        assertEquals(2, result.path("relations").findValuesAsText("kind").stream().filter("reads"::equals).count());
        boolean write = false, selectLine = false, updateLine = false, columnLine = false, ordersLine = false, customersLine = false, columnEvidenceLine = false, writeEvidenceLine = false;
        for (JsonNode relation : result.path("relations")) {
            if (relation.path("kind").asText().equals("writes")) { write = true; assertEquals(6, relation.path("location").path("line").asInt()); }
            if (relation.path("kind").asText().equals("reads")) assertTrue(relation.path("location").path("line").asInt() <= 5);
        }
        for (JsonNode symbol : result.path("symbols")) {
            if (symbol.path("kind").asText().equals("query") && symbol.path("location").path("line").asInt() == 2) selectLine = true;
            if (symbol.path("kind").asText().equals("query") && symbol.path("location").path("line").asInt() == 6) updateLine = true;
            if (symbol.path("qualifiedName").asText().equals("orders.id") && symbol.path("location").path("line").asInt() == 3) columnLine = true;
            if (symbol.path("qualifiedName").asText().equals("orders") && symbol.path("location").path("line").asInt() == 4) ordersLine = true;
            if (symbol.path("qualifiedName").asText().equals("customers") && symbol.path("location").path("line").asInt() == 5) customersLine = true;
        }
        for (JsonNode evidence : result.path("evidence")) {
            if (evidence.path("source").asText().startsWith("uses") && evidence.path("location").path("line").asInt() == 3) columnEvidenceLine = true;
            if (evidence.path("source").asText().startsWith("writes") && evidence.path("location").path("line").asInt() == 6) writeEvidenceLine = true;
        }
        assertTrue(write && selectLine && updateLine && columnLine && ordersLine && customersLine && columnEvidenceLine && writeEvidenceLine, result.toString());
    }

    @Test void sqlIdentifiersAndEvidenceChangeAcrossRevisions() {
        com.fasterxml.jackson.databind.node.ObjectNode firstInput = (com.fasterxml.jackson.databind.node.ObjectNode) input("SELECT id FROM orders");
        com.fasterxml.jackson.databind.node.ObjectNode secondInput = firstInput.deepCopy(); secondInput.put("revisionId", "next");
        JsonNode first = new SqlAnalyzer().analyze(firstInput, json), second = new SqlAnalyzer().analyze(secondInput, json);
        assertNotEquals(first.path("symbols").get(0).path("id"), second.path("symbols").get(0).path("id"));
        assertNotEquals(first.path("relations").get(0).path("id"), second.path("relations").get(0).path("id"));
        assertNotEquals(first.path("evidence").get(0).path("id"), second.path("evidence").get(0).path("id"));
    }

    private JsonNode input(String source) {
        com.fasterxml.jackson.databind.node.ObjectNode value = json.createObjectNode();
        value.put("projectId", "p"); value.put("revisionId", "r");
        value.putArray("artifacts").addObject().put("id", "a").put("path", "queries.sql").put("language", "sql");
        value.putObject("sources").put("queries.sql", source);
        return value;
    }
}
