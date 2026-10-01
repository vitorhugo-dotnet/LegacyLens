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
import java.io.File;
import java.nio.file.Path;
import javax.tools.ToolProvider;
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

    @Test void identifiersAndEdgesAreScopedToRevision() throws Exception {
        String source = "class Versioned { void target() {} void run() { target(); } }";
        com.fasterxml.jackson.databind.node.ObjectNode firstInput = (com.fasterxml.jackson.databind.node.ObjectNode) input("Versioned.java", source);
        com.fasterxml.jackson.databind.node.ObjectNode secondInput = firstInput.deepCopy(); secondInput.put("revisionId", "next");
        JsonNode first = new JavaAnalyzer().analyze(firstInput, json), second = new JavaAnalyzer().analyze(secondInput, json);
        assertEquals(1, first.path("relations").size());
        assertEquals(1, second.path("relations").size());
        assertNotEquals(first.path("symbols").get(0).path("id").asText(), second.path("symbols").get(0).path("id").asText());
        assertNotEquals(first.path("relations").get(0).path("id").asText(), second.path("relations").get(0).path("id").asText());
        assertNotEquals(first.path("evidence").get(0).path("id").asText(), second.path("evidence").get(0).path("id").asText());
        for (JsonNode relation : second.path("relations")) {
            assertTrue(hasSymbol(second, relation.path("fromId").asText()));
            assertTrue(hasSymbol(second, relation.path("toId").asText()));
        }
    }

    @Test void callsInConstructorAndInitializerHaveOwners() throws Exception {
        String source = "class Owners { static void target() {} Owners() { target(); } { target(); } static { target(); } }";
        JsonNode result = new JavaAnalyzer().analyze(input("Owners.java", source), json);
        assertEquals(3, result.path("relations").size(), result.toString());
        for (JsonNode relation : result.path("relations")) assertTrue(hasSymbol(result, relation.path("fromId").asText()), result.toString());
    }

    @Test void fieldInitializerWithoutExecutableOwnerReportsCoverage() throws Exception {
        JsonNode result = new JavaAnalyzer().analyze(input("Field.java", "class Field { static Object make() { return null; } Object value = make(); }"), json);
        assertTrue(result.path("diagnostics").toString().contains("java.unowned_call"), result.toString());
    }

    @Test void compiledClassDirectoriesAndPathListResolveSymbols() throws Exception {
        Path root = Files.createTempDirectory("legacylens classes with spaces ");
        Path other = Files.createTempDirectory("legacylens empty classes ");
        try {
            Path source = root.resolve("Library.java");
            Files.write(source, "package sample; public class Library { public static void work() {} }".getBytes(StandardCharsets.UTF_8));
            assertEquals(0, ToolProvider.getSystemJavaCompiler().run(null, null, null, "-Xlint:-options", "-source", "8", "-target", "8", "-d", root.toString(), source.toString()));
            com.fasterxml.jackson.databind.node.ObjectNode value = (com.fasterxml.jackson.databind.node.ObjectNode) input("Caller.java", "package sample; class Caller { void run() { Library.work(); } }");
            value.putArray("classpath").add(other.toString() + File.pathSeparator + root.toString());
            JsonNode result = new JavaAnalyzer().analyze(value, json);
            assertTrue(result.path("relations").toString().contains("resolved"), result.toString());
            assertFalse(result.path("diagnostics").toString().contains("classpath_unsupported"), result.toString());
        } finally {
            Files.deleteIfExists(root.resolve("sample").resolve("Library.class"));
            Files.deleteIfExists(root.resolve("sample")); Files.deleteIfExists(root.resolve("Library.java")); Files.deleteIfExists(root); Files.deleteIfExists(other);
        }
    }

    @Test void unavailableClasspathHasSanitizedDiagnostic() throws Exception {
        com.fasterxml.jackson.databind.node.ObjectNode value = (com.fasterxml.jackson.databind.node.ObjectNode) input("Simple.java", "class Simple { void run() {} }");
        value.putArray("classpath").add("private-token.jar");
        JsonNode result = new JavaAnalyzer().analyze(value, json);
        assertTrue(result.path("diagnostics").toString().contains("java.classpath_unsupported"));
        assertFalse(result.toString().contains("private-token"));
    }

    private static boolean hasSymbol(JsonNode result, String id) {
        for (JsonNode symbol : result.path("symbols")) if (symbol.path("id").asText().equals(id)) return true;
        return false;
    }

    private JsonNode input(String path, String source) throws Exception {
        com.fasterxml.jackson.databind.node.ObjectNode value = json.createObjectNode();
        value.put("projectId", "p"); value.put("revisionId", "r"); value.put("root", ".");
        value.putArray("artifacts").addObject().put("id", "a").put("path", path).put("language", "java");
        value.putObject("sources").put(path, source);
        return value;
    }
}
