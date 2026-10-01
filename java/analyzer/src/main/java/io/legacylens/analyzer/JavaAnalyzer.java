package io.legacylens.analyzer;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.github.javaparser.JavaParser;
import com.github.javaparser.ParseResult;
import com.github.javaparser.ParserConfiguration;
import com.github.javaparser.ast.CompilationUnit;
import com.github.javaparser.ast.body.MethodDeclaration;
import com.github.javaparser.ast.expr.MethodCallExpr;
import com.github.javaparser.resolution.declarations.ResolvedMethodDeclaration;
import com.github.javaparser.symbolsolver.JavaSymbolSolver;
import com.github.javaparser.symbolsolver.resolution.typesolvers.CombinedTypeSolver;
import com.github.javaparser.symbolsolver.resolution.typesolvers.JavaParserTypeSolver;
import com.github.javaparser.symbolsolver.resolution.typesolvers.JarTypeSolver;
import com.github.javaparser.symbolsolver.resolution.typesolvers.ReflectionTypeSolver;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.stream.Stream;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.TreeMap;

/** Java syntax and symbol resolution using JavaParser; unresolved calls stay explicit. */
public final class JavaAnalyzer {
    public ObjectNode analyze(JsonNode input, ObjectMapper mapper) {
        ObjectNode result = AnalyzerMain.empty(mapper);
        Path sourceRoot = null;
        try {
            sourceRoot = Files.createTempDirectory("legacylens-java-");
            Map<String, JsonNode> artifacts = new TreeMap<>();
            Map<String, CompilationUnit> units = new TreeMap<>();
            JavaParser syntaxParser = new JavaParser(new ParserConfiguration().setLanguageLevel(ParserConfiguration.LanguageLevel.JAVA_21));
            for (JsonNode artifact : input.path("artifacts")) {
                if (!"java".equalsIgnoreCase(artifact.path("language").asText())) continue;
                String path = artifact.path("path").asText();
                JsonNode source = input.path("sources").get(path);
                if (source == null || !source.isTextual()) continue;
                ParseResult<CompilationUnit> parsed = syntaxParser.parse(source.asText());
                if (!parsed.getResult().isPresent()) { AnalyzerMain.diagnostic(result, mapper, "java.parse", "Java source could not be parsed safely.", path); continue; }
                CompilationUnit unit = parsed.getResult().get();
                if (!parsed.isSuccessful()) AnalyzerMain.diagnostic(result, mapper, "java.unsupported_syntax", "Some Java syntax is unsupported by the configured parser.", path);
                String packagePath = unit.getPackageDeclaration().map(p -> p.getNameAsString().replace('.', '/')).orElse("");
                Path destination = sourceRoot.resolve(packagePath).resolve(Paths.get(path).getFileName().toString()).normalize();
                if (!destination.startsWith(sourceRoot)) continue;
                Files.createDirectories(destination.getParent());
                Files.write(destination, source.asText().getBytes(StandardCharsets.UTF_8));
                artifacts.put(path, artifact); units.put(path, unit);
            }
            CombinedTypeSolver solver = new CombinedTypeSolver();
            solver.add(new ReflectionTypeSolver()); solver.add(new JavaParserTypeSolver(sourceRoot));
            for (JsonNode root : input.path("sourceRoots")) {
                Path path = Paths.get(root.asText());
                if (Files.isDirectory(path)) solver.add(new JavaParserTypeSolver(path));
                else AnalyzerMain.diagnostic(result, mapper, "java.source_root_unavailable", "A configured Java source root is unavailable.", "");
            }
            for (JsonNode entry : input.path("classpath")) {
                Path path = Paths.get(entry.asText());
                if (Files.isRegularFile(path) && path.toString().toLowerCase().endsWith(".jar")) solver.add(new JarTypeSolver(path));
                else AnalyzerMain.diagnostic(result, mapper, "java.classpath_unsupported", "A configured classpath entry is unavailable or unsupported; only JAR files are supported.", "");
            }
            JavaParser parser = new JavaParser(new ParserConfiguration().setLanguageLevel(ParserConfiguration.LanguageLevel.JAVA_21).setSymbolResolver(new JavaSymbolSolver(solver)));
            Map<String, String> declared = new HashMap<>();
            for (Map.Entry<String, CompilationUnit> entry : units.entrySet()) {
                String path = entry.getKey();
                ParseResult<CompilationUnit> parsed = parser.parse(input.path("sources").path(path).asText());
                if (!parsed.getResult().isPresent()) continue;
                CompilationUnit unit = parsed.getResult().get();
                JsonNode artifact = artifacts.get(path);
                for (MethodDeclaration method : unit.findAll(MethodDeclaration.class)) {
                    try {
                        ResolvedMethodDeclaration resolved = method.resolve();
                        String name = resolved.getQualifiedName(); String descriptor = descriptor(resolved);
                        String id = AnalyzerMain.symbol(result, mapper, input, artifact, name, descriptor, "method", method.getBegin().map(p -> p.line).orElse(1)).path("id").asText();
                        declared.put(name + descriptor, id);
                    } catch (RuntimeException error) {
                        AnalyzerMain.diagnostic(result, mapper, "java.unresolved_declaration", "A Java declaration could not be resolved with the available source roots and classpath.", path);
                    }
                }
            }
            for (Map.Entry<String, CompilationUnit> entry : units.entrySet()) {
                String path = entry.getKey();
                ParseResult<CompilationUnit> parsed = parser.parse(input.path("sources").path(path).asText());
                if (!parsed.getResult().isPresent()) continue;
                CompilationUnit unit = parsed.getResult().get();
                JsonNode artifact = artifacts.get(path);
                for (MethodCallExpr call : unit.findAll(MethodCallExpr.class)) {
                    int line = call.getBegin().map(p -> p.line).orElse(1);
                    MethodDeclaration caller = call.findAncestor(MethodDeclaration.class).orElse(null);
                    if (caller == null) continue;
                    String callerId;
                    try { ResolvedMethodDeclaration owner = caller.resolve(); callerId = AnalyzerMain.id(input.path("projectId").asText(), path, owner.getQualifiedName(), descriptor(owner)); }
                    catch (RuntimeException error) { continue; }
                    try {
                        ResolvedMethodDeclaration target = call.resolve();
                        String name = target.getQualifiedName(), descriptor = descriptor(target);
                        if (isReflective(name)) {
                            AnalyzerMain.relation(result, mapper, callerId, "", "calls", "dynamic", path, line);
                            continue;
                        }
                        String targetId = declared.get(name + descriptor);
                        if (targetId == null) {
                            // An external reference is anchored to its caller's artifact so the index remains valid.
                            targetId = AnalyzerMain.symbol(result, mapper, input, artifact, name, descriptor, "external_method", line).path("id").asText();
                            declared.put(name + descriptor, targetId);
                        }
                        AnalyzerMain.relation(result, mapper, callerId, targetId, "calls", "resolved", path, line);
                    } catch (RuntimeException error) {
                        AnalyzerMain.relation(result, mapper, callerId, "", "calls", "unresolved", path, line);
                        AnalyzerMain.diagnostic(result, mapper, "java.unresolved_symbol", "A Java call target could not be resolved with the available source roots and classpath.", path);
                    }
                }
            }
        } catch (Exception error) {
            AnalyzerMain.diagnostic(result, mapper, "java.analysis", "Java analysis could not be completed safely.", "");
        } finally {
            if (sourceRoot != null) try (Stream<Path> stream = Files.walk(sourceRoot)) { List<Path> paths = new ArrayList<>(); stream.forEach(paths::add); paths.sort(Comparator.reverseOrder()); for (Path path : paths) Files.deleteIfExists(path); } catch (Exception ignored) { }
        }
        return result;
    }
    private static String descriptor(ResolvedMethodDeclaration method) {
        StringBuilder value = new StringBuilder("(");
        for (int i = 0; i < method.getNumberOfParams(); i++) { if (i > 0) value.append(','); value.append(method.getParam(i).getType().describe()); }
        return value.append(')').append(method.getReturnType().describe()).toString();
    }
    private static boolean isReflective(String name) {
        return name.equals("java.lang.Class.forName") || name.equals("java.lang.Class.newInstance")
                || name.equals("java.lang.reflect.Method.invoke") || name.equals("java.lang.reflect.Constructor.newInstance")
                || name.equals("java.lang.invoke.MethodHandle.invoke") || name.equals("java.lang.invoke.MethodHandle.invokeExact")
                || name.equals("java.lang.invoke.MethodHandle.invokeWithArguments");
    }
}
