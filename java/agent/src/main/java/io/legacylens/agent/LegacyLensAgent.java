package io.legacylens.agent;

import io.legacylens.agent.instrumentation.JdbcAdvice;
import io.legacylens.agent.instrumentation.MethodAdvice;
import io.legacylens.agent.instrumentation.PrepareStatementAdvice;
import io.legacylens.agent.instrumentation.PreparedStatementAdvice;
import io.legacylens.agent.instrumentation.ServletAdvice;
import net.bytebuddy.agent.builder.AgentBuilder;
import net.bytebuddy.asm.Advice;
import net.bytebuddy.jar.asm.ClassReader;
import net.bytebuddy.jar.asm.ClassVisitor;
import net.bytebuddy.jar.asm.MethodVisitor;
import net.bytebuddy.jar.asm.Opcodes;
import java.io.File;
import java.io.InputStream;
import java.lang.instrument.Instrumentation;
import java.lang.reflect.Method;
import java.nio.file.Files;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Set;
import java.util.WeakHashMap;
import java.util.jar.JarEntry;
import java.util.jar.JarFile;
import java.util.jar.JarOutputStream;
import static net.bytebuddy.matcher.ElementMatchers.*;

public final class LegacyLensAgent {
    private static volatile AgentConfig config;
    private static volatile AgentTransport transport;
    private static volatile boolean installed;
    private static JarFile bootstrapBridgeJar;
    private static final String BRIDGE_NAME = "io.legacylens.agent.bridge.AgentBridge";
    private static final ThreadLocal<Integer> jdbcDepth = new ThreadLocal<Integer>();
    private static final Map<Object, String> preparedSql = Collections.synchronizedMap(new WeakHashMap<Object, String>());

    private LegacyLensAgent() { }

    public static void premain(String options, Instrumentation instrumentation) {
        try {
            config = AgentConfig.load(options);
            transport = AgentTransport.start(config);
            install(instrumentation, config);
            installed = true;
        } catch (Exception failure) {
            System.err.println("LegacyLens agent configuration failed: " + failure.getClass().getSimpleName());
        }
    }

    static synchronized void installForTesting(Instrumentation instrumentation, AgentTransport testTransport, AgentConfig testConfig) {
        config = testConfig;
        transport = testTransport;
        if (!installed) {
            try {
                install(instrumentation, testConfig);
                installed = true;
            } catch (Exception failure) {
                throw new IllegalStateException("Could not install LegacyLens test instrumentation", failure);
            }
        }
    }

    private static void install(Instrumentation instrumentation, final AgentConfig agentConfig) throws Exception {
        installBootstrapBridge(instrumentation);
        new AgentBuilder.Default().ignore(nameStartsWith("io.legacylens.agent.").or(nameStartsWith("net.bytebuddy.")).or(nameStartsWith("java.")))
          .type(hasSuperType(named("javax.servlet.Servlet")).or(hasSuperType(named("jakarta.servlet.Servlet"))))
          .transform((builder, type, loader, module, protectionDomain) -> builder.visit(Advice.to(ServletAdvice.class).on(named("service").and(takesArguments(2)))))
          .type(nameStartsWith("com.mysql.").or(nameStartsWith("org.mariadb.")))
          .transform((builder, type, loader, module, protectionDomain) -> builder
              .visit(Advice.to(JdbcAdvice.class).on(namedOneOf("execute", "executeQuery", "executeUpdate").and(takesArguments(1))))
              .visit(Advice.to(PreparedStatementAdvice.class).on(namedOneOf("execute", "executeQuery", "executeUpdate").and(takesArguments(0))))
              .visit(Advice.to(PrepareStatementAdvice.class).on(namedOneOf("prepareStatement", "prepareCall").and(takesArguments(1)))))
          .type(new net.bytebuddy.matcher.ElementMatcher<net.bytebuddy.description.type.TypeDescription>() {
              public boolean matches(net.bytebuddy.description.type.TypeDescription type) {
                  String name = type.getName();
                  return instrumentableApplicationType(name, agentConfig.packages);
              }
          })
          .transform((builder, type, loader, module, protectionDomain) -> builder.visit(Advice.to(MethodAdvice.class).on(isMethod().and(not(isAbstract())).and(not(isNative())))))
          .with(AgentBuilder.RedefinitionStrategy.RETRANSFORMATION).installOn(instrumentation);
    }

    static boolean instrumentableApplicationType(String name, Set<String> packages) {
        if (name.startsWith("io.legacylens.agent.") || name.startsWith("java.") || name.startsWith("javax.")
                || name.startsWith("jakarta.") || name.startsWith("net.bytebuddy.")) return false;
        for (String allowed : packages) if (name.equals(allowed) || name.startsWith(allowed + ".")) return true;
        return false;
    }

    private static synchronized void installBootstrapBridge(Instrumentation instrumentation) throws Exception {
        if (bootstrapBridgeJar == null) {
            String entryName = BRIDGE_NAME.replace('.', '/') + ".class";
            File bridgeFile = Files.createTempFile("legacylens-bootstrap-bridge-", ".jar").toFile();
            try (InputStream bytes = LegacyLensAgent.class.getClassLoader().getResourceAsStream(entryName);
                 JarOutputStream jar = new JarOutputStream(Files.newOutputStream(bridgeFile.toPath()))) {
                if (bytes == null) throw new IllegalStateException("Bootstrap bridge class resource is missing");
                jar.putNextEntry(new JarEntry(entryName));
                byte[] buffer = new byte[4096];
                for (int count; (count = bytes.read(buffer)) >= 0; ) if (count > 0) jar.write(buffer, 0, count);
                jar.closeEntry();
            }
            bootstrapBridgeJar = new JarFile(bridgeFile);
            instrumentation.appendToBootstrapClassLoaderSearch(bootstrapBridgeJar);
            bridgeFile.deleteOnExit();
        }
        Class<?> bridge = Class.forName(BRIDGE_NAME, true, null);
        Method bind = bridge.getMethod("bind", Class.class);
        bind.invoke(null, LegacyLensAgent.class);
    }

    public static boolean servletEnter(Object request) {
        String parent = null, trace = null;
        try {
            String traceparent = (String) request.getClass().getMethod("getHeader", String.class).invoke(request, "traceparent");
            if (TraceContext.validTraceparent(traceparent)) { trace = traceparent.substring(3, 35); parent = traceparent.substring(36, 52); }
        } catch (Exception ignored) { }
        if (trace == null) return false;
        AgentConfig currentConfig = config; AgentTransport currentTransport = transport;
        if (currentConfig == null || currentTransport == null) return false;
        AgentTransport.TraceState state = currentTransport.acquireTrace(trace, currentConfig.producerId);
        if (state == null) return false;
        TraceContext.begin(trace, parent, state);
        Map<String, String> metadata = new LinkedHashMap<String, String>();
        metadata.put("http.request_span",parent);
        try {
            String method = String.valueOf(request.getClass().getMethod("getMethod").invoke(request));
            if (method.matches("[A-Z]{1,12}")) metadata.put("http.method", method);
        } catch (Exception ignored) { }
        Event event = emit("http.server", metadata);
        if (event != null) TraceContext.pushEvent(event.eventId);
        return true;
    }

    public static void servletExit(boolean active) {
        if (!active) return;
        try { emit("http.server.end", new LinkedHashMap<String, String>()); }
        finally {
            TraceContext ended = TraceContext.end();
            AgentTransport currentTransport = transport;
            if (ended != null && ended.state != null && currentTransport != null) currentTransport.releaseTrace(ended.state);
        }
    }

    public static boolean methodEnter(String type, String name, String descriptor, Class<?> owner) {
        TraceContext trace = TraceContext.current();
        if (trace == null) return false;
        Event event = emit("method.start", identity(type, name, descriptor, owner));
        if (event != null) TraceContext.pushEvent(event.eventId);
        return event != null;
    }

    public static void methodExit(String type, String name, String descriptor, Class<?> owner, Throwable thrown, boolean active) {
        if (!active) return;
        TraceContext trace = TraceContext.current();
        try { emit(thrown == null ? "method.end" : "method.error", identity(type, name, descriptor, owner)); }
        finally { if (trace != null) TraceContext.popEvent(); }
    }

    static Map<String, String> identity(String type, String name, String descriptor, Class<?> owner) {
        Map<String, String> metadata = new LinkedHashMap<String, String>();
        metadata.put("code.class", type); metadata.put("code.method", name); metadata.put("code.descriptor", descriptor);
        metadata.put("code.deployment", AgentConfig.deploymentIdentity("deployment", owner.getClassLoader()));
        Integer line = firstLine(owner, name, descriptor);
        if (line == null) metadata.put("code.line_missing", "true"); else metadata.put("code.line", String.valueOf(line));
        return metadata;
    }

    private static Integer firstLine(final Class<?> owner, final String methodName, final String descriptor) {
        String resource = "/" + owner.getName().replace('.', '/') + ".class";
        try (InputStream bytes = owner.getResourceAsStream(resource)) {
            if (bytes == null) return null;
            final int[] line = {0};
            new ClassReader(bytes).accept(new ClassVisitor(Opcodes.ASM9) {
                @Override public MethodVisitor visitMethod(int access, String name, String desc, String signature, String[] exceptions) {
                    if (!methodName.equals(name) || !descriptor.equals(desc)) return null;
                    return new MethodVisitor(Opcodes.ASM9) {
                        @Override public void visitLineNumber(int lineNumber, net.bytebuddy.jar.asm.Label start) { if (line[0] == 0) line[0] = lineNumber; }
                    };
                }
            }, ClassReader.SKIP_FRAMES);
            return line[0] == 0 ? null : line[0];
        } catch (Exception ignored) { return null; }
    }

    public static boolean jdbcEnter(String operation, String sql) {
        Integer depth = jdbcDepth.get(); int current = depth == null ? 0 : depth; jdbcDepth.set(current + 1);
        if (current > 0) return false;
        String safe = Sanitizer.sql(sql);
        Map<String, String> metadata = new LinkedHashMap<String, String>(); metadata.put("db.operation", operation); metadata.put("db.system", "jdbc");
        if (!safe.isEmpty()) metadata.put("sql", safe);
        emit("db." + operation, metadata);
        return true;
    }

    public static void jdbcExit(String operation, String sql, boolean outer) {
        Integer depth = jdbcDepth.get(); if (depth == null) return; if (depth <= 1) jdbcDepth.remove(); else jdbcDepth.set(depth - 1);
    }

    public static void preparedStatement(Object statement, String sql) {
        if (statement == null) return;
        String safe = Sanitizer.sql(sql); if (!safe.isEmpty()) preparedSql.put(statement, safe);
    }

    public static boolean preparedExecute(Object statement, String method) {
        String sql = preparedSql.get(statement);
        String operation = method.equals("executeQuery") ? "query" : method.equals("executeUpdate") ? "update" : "execute";
        return jdbcEnter(operation, sql);
    }

    private static Event emit(String kind, Map<String, String> metadata) {
        AgentConfig currentConfig = config; AgentTransport currentTransport = transport; TraceContext trace = TraceContext.current();
        if (currentConfig == null || currentTransport == null || trace == null || trace.state == null) return null;
        return currentTransport.emit(trace.state, currentConfig.projectId, currentConfig.revision, kind, metadata, TraceContext.parentEventId());
    }

    static void beginTraceForTesting(String traceId) {
        AgentConfig currentConfig = config; AgentTransport currentTransport = transport;
        AgentTransport.TraceState state = currentConfig == null || currentTransport == null ? null : currentTransport.acquireTrace(traceId, currentConfig.producerId);
        if (state != null) TraceContext.begin(traceId, null, state);
    }
    static void endTraceForTesting() {
        TraceContext ended = TraceContext.end(); AgentTransport currentTransport = transport;
        if (ended != null && ended.state != null && currentTransport != null) currentTransport.releaseTrace(ended.state);
    }
}
