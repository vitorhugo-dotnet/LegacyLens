package io.legacylens.analyzer;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Paths;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.io.PrintStream;
import static org.junit.jupiter.api.Assertions.*;

class JavaAnalyzerTest {
    private final ObjectMapper json = new ObjectMapper();

    @Test void overloadsResolveToDistinctDescriptorsAndReflectionIsDynamic() throws Exception {
        String source = "package sample; class Overloads { void call(String s) {} void call(int n) {} void run() { call(1); call(\"x\"); Class.forName(\"secret\"); } }";
        JsonNode result = new JavaAnalyzer().analyze(input("Overloads.java", source), json);
        long resolved = 0, dynamic = 0;
        for (JsonNode relation : result.path("relations")) {
            if ("resolved".equals(relation.path("resolution").asText())) resolved++;
            if ("dynamic".equals(relation.path("resolution").asText())) dynamic++;
        }
        assertEquals(2, resolved);
        assertEquals(1, dynamic);
        assertEquals(result.path("relations").size(), result.path("evidence").size());
        String symbols = result.path("symbols").toString();
        assertTrue(symbols.contains("(int)void"));
        assertTrue(symbols.contains("(java.lang.String)void"));
        assertFalse(result.toString().contains("secret"));
    }

    @Test void unknownDispatchIsConservativeAndJava21Parses() throws Exception {
        String source = "package sample; interface Service { void go(); } class Impl implements Service { public void go() {} } record Modern(String value) { void run(Service service) { service.go(); } }";
        JsonNode result = new JavaAnalyzer().analyze(input("Modern.java", source), json);
        assertFalse(result.path("symbols").isEmpty());
        assertTrue(result.toString().contains("go"));
        String implementationId = "";
        for (JsonNode symbol : result.path("symbols")) if (symbol.path("qualifiedName").asText().equals("sample.Impl.go")) implementationId = symbol.path("id").asText();
        for (JsonNode relation : result.path("relations")) assertNotEquals(implementationId, relation.path("toId").asText());
    }

    @Test void java21FixtureParsesWithoutCompilingItsSource() throws Exception {
        String source = new String(Files.readAllBytes(Paths.get("..", "..", "fixtures", "static", "java", "ReflectiveCalls.java")), StandardCharsets.UTF_8);
        JsonNode result = new JavaAnalyzer().analyze(input("ReflectiveCalls.java", source), json);
        assertTrue(result.path("diagnostics").toString().contains("java.unresolved_symbol") || result.path("symbols").size() > 0);
        assertFalse(result.path("diagnostics").toString().contains("java.parse"));
        assertFalse(result.path("diagnostics").toString().contains("java.unsupported_syntax"));
    }

    @Test void configuredSourceRootResolvesCallWithoutClaimingVirtualDispatch() throws Exception {
        java.nio.file.Path root = Files.createTempDirectory("legacylens root with spaces ");
        try {
            Files.createDirectories(root.resolve("sample"));
            Files.write(root.resolve("sample").resolve("Dependency.java"), "package sample; class Dependency { static void work() {} }".getBytes(StandardCharsets.UTF_8));
            com.fasterxml.jackson.databind.node.ObjectNode value = (com.fasterxml.jackson.databind.node.ObjectNode) input("Caller.java", "package sample; class Caller { void run() { Dependency.work(); } }");
            value.putArray("sourceRoots").add(root.toString());
            JsonNode result = new JavaAnalyzer().analyze(value, json);
            assertTrue(result.path("relations").toString().contains("resolved"), result.toString());
            assertFalse(result.path("diagnostics").toString().contains("java.unresolved_symbol"), result.toString());
        } finally {
            Files.deleteIfExists(root.resolve("sample").resolve("Dependency.java"));
            Files.deleteIfExists(root.resolve("sample")); Files.deleteIfExists(root);
        }
    }

    @Test void reflectiveMethodInvokeKeepsTargetDynamic() throws Exception {
        JsonNode result = new JavaAnalyzer().analyze(input("Reflective.java", "class Reflective { void run(java.lang.reflect.Method method, Object receiver) throws Exception { method.invoke(receiver); } }"), json);
        boolean dynamic = false;
        for (JsonNode relation : result.path("relations")) if (relation.path("resolution").asText().equals("dynamic")) dynamic = true;
        assertTrue(dynamic, result.toString());
    }

    @Test void workerWritesOneSanitizedResultPerInputLine() throws Exception {
        String first = json.writeValueAsString(input("Overloads.java", "class Overloads { void run() {} }"));
        InputStream previousIn = System.in; PrintStream previousOut = System.out;
        ByteArrayOutputStream output = new ByteArrayOutputStream();
        try {
            System.setIn(new ByteArrayInputStream((first + "\n{private-token}\n").getBytes(StandardCharsets.UTF_8)));
            System.setOut(new PrintStream(output, true, "UTF-8"));
            AnalyzerMain.main(new String[0]);
        } finally { System.setIn(previousIn); System.setOut(previousOut); }
        String[] lines = output.toString("UTF-8").trim().split("\\R");
        assertEquals(2, lines.length);
        assertTrue(json.readTree(lines[0]).path("symbols").size() > 0);
        assertEquals("worker.invalid_input", json.readTree(lines[1]).path("diagnostics").get(0).path("code").asText());
        assertFalse(output.toString("UTF-8").contains("private-token"));
    }

    private JsonNode input(String path, String source) throws Exception {
        com.fasterxml.jackson.databind.node.ObjectNode value = json.createObjectNode();
        value.put("projectId", "p"); value.put("revisionId", "r"); value.put("root", ".");
        value.putArray("artifacts").addObject().put("id", "a").put("path", path).put("language", "java");
        value.putObject("sources").put(path, source);
        return value;
    }
}
