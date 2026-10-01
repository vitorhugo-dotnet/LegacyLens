package io.legacylens.analyzer;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;
import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.io.OutputStreamWriter;
import java.io.PrintWriter;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;

/** One JSON AnalysisInput envelope per line; one AnalysisResult per line. */
public final class AnalyzerMain {
    private AnalyzerMain() {}
    public static void main(String[] args) throws Exception {
        ObjectMapper mapper = new ObjectMapper();
        BufferedReader in = new BufferedReader(new InputStreamReader(System.in, StandardCharsets.UTF_8));
        PrintWriter out = new PrintWriter(new OutputStreamWriter(System.out, StandardCharsets.UTF_8), true);
        String line;
        while ((line = in.readLine()) != null) {
            try {
                JsonNode input = mapper.readTree(line);
                ObjectNode result = empty(mapper);
                merge(result, new JavaAnalyzer().analyze(input, mapper));
                merge(result, new SqlAnalyzer().analyze(input, mapper));
                out.println(mapper.writeValueAsString(result));
            } catch (Exception ignored) {
                ObjectNode result = empty(mapper);
                diagnostic(result, mapper, "worker.invalid_input", "Analysis input could not be processed safely.", "");
                out.println(mapper.writeValueAsString(result));
            }
        }
    }

    static ObjectNode empty(ObjectMapper mapper) {
        ObjectNode result = mapper.createObjectNode();
        result.putArray("symbols"); result.putArray("relations"); result.putArray("evidence"); result.putArray("diagnostics");
        return result;
    }
    private static void merge(ObjectNode into, ObjectNode from) {
        for (String key : new String[]{"symbols", "relations", "evidence", "diagnostics"})
            for (JsonNode item : from.path(key)) ((ArrayNode) into.get(key)).add(item);
    }
    static String id(String... parts) {
        try {
            MessageDigest hash = MessageDigest.getInstance("SHA-256");
            for (String part : parts) { hash.update(part.getBytes(StandardCharsets.UTF_8)); hash.update((byte) 0); }
            byte[] bytes = hash.digest(); StringBuilder value = new StringBuilder();
            for (byte b : bytes) value.append(String.format("%02x", b & 0xff));
            return value.toString();
        } catch (Exception impossible) { throw new IllegalStateException(impossible); }
    }
    static ObjectNode symbol(ObjectNode result, ObjectMapper mapper, JsonNode input, JsonNode artifact, String name, String descriptor, String kind, int line) {
        String path = artifact.path("path").asText();
        String symbolId = id(input.path("projectId").asText(), input.path("revisionId").asText(), path, name, descriptor);
        for (JsonNode existing : result.path("symbols")) if (symbolId.equals(existing.path("id").asText())) return (ObjectNode) existing;
        ObjectNode value = ((ArrayNode) result.get("symbols")).addObject();
        value.put("id", symbolId);
        value.put("projectId", input.path("projectId").asText()); value.put("revisionId", input.path("revisionId").asText());
        value.put("artifactId", artifact.path("id").asText()); value.put("path", path);
        value.put("qualifiedName", name); value.put("descriptor", descriptor); value.put("kind", kind);
        ObjectNode location = value.putObject("location"); location.put("path", path); location.put("line", Math.max(1, line)); location.put("column", 1);
        return value;
    }
    static void relation(ObjectNode result, ObjectMapper mapper, String from, String to, String kind, String resolution, String path, int line) {
        String relationId = id(from, kind, to, String.valueOf(line));
        for (JsonNode existing : result.path("relations")) if (relationId.equals(existing.path("id").asText())) return;
        ObjectNode value = ((ArrayNode) result.get("relations")).addObject();
        value.put("id", relationId); value.put("fromId", from);
        if (!to.isEmpty()) value.put("toId", to);
        String evidenceId = id(from, kind, to, path, String.valueOf(line), "evidence");
        value.put("kind", kind); value.putArray("evidenceIds").add(evidenceId); value.put("resolution", resolution); value.put("layer", "static");
        ObjectNode location = value.putObject("location"); location.put("path", path); location.put("line", Math.max(1, line)); location.put("column", 1);
        ObjectNode evidence = ((ArrayNode) result.get("evidence")).addObject();
        evidence.put("id", evidenceId); evidence.put("kind", "source"); evidence.put("source", kind + " AST reference");
        ObjectNode evidenceLocation = evidence.putObject("location"); evidenceLocation.put("path", path); evidenceLocation.put("line", Math.max(1, line)); evidenceLocation.put("column", 1);
    }
    static void diagnostic(ObjectNode result, ObjectMapper mapper, String code, String message, String path) {
        String diagnosticId = id(code, path);
        for (JsonNode existing : result.path("diagnostics")) if (diagnosticId.equals(existing.path("id").asText())) return;
        ObjectNode value = ((ArrayNode) result.get("diagnostics")).addObject();
        value.put("id", diagnosticId); value.put("code", code); value.put("message", message); value.put("severity", "warning");
        if (!path.isEmpty()) { ObjectNode location = value.putObject("location"); location.put("path", path); location.put("line", 1); location.put("column", 1); }
    }
}
