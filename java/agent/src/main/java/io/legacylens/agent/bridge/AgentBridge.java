package io.legacylens.agent.bridge;

import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Method;

/** Bootstrap-visible, dependency-free dispatch into the system-loaded agent. */
public final class AgentBridge {
    private static volatile Method servletEnter;
    private static volatile Method servletExit;
    private static volatile Method methodEnter;
    private static volatile Method methodExit;
    private static volatile Method jdbcEnter;
    private static volatile Method jdbcExit;
    private static volatile Method prepareStatement;
    private static volatile Method preparedExecute;
    private static volatile Method endpointEnter;
    private static volatile Method endpointExit;
    private static volatile Method facesActionEnter;
    private static volatile Method facesActionExit;

    private AgentBridge() { }

    public static void bind(Class<?> agent) throws NoSuchMethodException {
        servletEnter = agent.getMethod("servletEnter", Object.class);
        servletExit = agent.getMethod("servletExit", boolean.class);
        methodEnter = agent.getMethod("methodEnter", String.class, String.class, String.class, Class.class);
        methodExit = agent.getMethod("methodExit", String.class, String.class, String.class, Class.class, Throwable.class, boolean.class);
        jdbcEnter = agent.getMethod("jdbcEnter", String.class, String.class);
        jdbcExit = agent.getMethod("jdbcExit", String.class, String.class, boolean.class);
        prepareStatement = agent.getMethod("preparedStatement", Object.class, String.class);
        preparedExecute = agent.getMethod("preparedExecute", Object.class, String.class);
        endpointEnter = agent.getMethod("endpointEnter", Class.class);
        endpointExit = agent.getMethod("endpointExit", Throwable.class);
        facesActionEnter = agent.getMethod("facesActionEnter", Class.class, String.class);
        facesActionExit = agent.getMethod("facesActionExit", Class.class, String.class, Throwable.class);
    }

    public static boolean servletEnter(Object request) { return booleanResult(call(servletEnter, request)); }
    public static void servletExit(boolean active) { call(servletExit, active); }
    public static boolean methodEnter(String type, String name, String descriptor, Class<?> owner) { return booleanResult(call(methodEnter, type, name, descriptor, owner)); }
    public static void methodExit(String type, String name, String descriptor, Class<?> owner, Throwable thrown, boolean active) { call(methodExit, type, name, descriptor, owner, thrown, active); }
    public static boolean jdbcEnter(String operation, String sql) { return booleanResult(call(jdbcEnter, operation, sql)); }
    public static void jdbcExit(String operation, String sql, boolean outer) { call(jdbcExit, operation, sql, outer); }
    public static void preparedStatement(Object statement, String sql) { call(prepareStatement, statement, sql); }
    public static boolean preparedExecute(Object statement, String method) { return booleanResult(call(preparedExecute, statement, method)); }
    public static boolean endpointEnter(Class<?> endpoint) { return booleanResult(call(endpointEnter, endpoint)); }
    public static void endpointExit(Throwable thrown) { call(endpointExit, thrown); }
    public static boolean facesActionEnter(Class<?> listener, String method) { return booleanResult(call(facesActionEnter, listener, method)); }
    public static void facesActionExit(Class<?> listener, String method, Throwable thrown) { call(facesActionExit, listener, method, thrown); }

    private static Object call(Method method, Object... arguments) {
        if (method == null) return null;
        try { return method.invoke(null, arguments); }
        catch (IllegalAccessException | InvocationTargetException | RuntimeException ignored) { return null; }
    }

    private static boolean booleanResult(Object result) { return Boolean.TRUE.equals(result); }
}
