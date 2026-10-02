package io.legacylens.agent.instrumentation;

import net.bytebuddy.asm.Advice;

/** Emits an endpoint marker while the existing servlet trace is active. */
public final class EndpointAdvice {
    private EndpointAdvice() { }

    @Advice.OnMethodEnter(suppress = Throwable.class)
    public static boolean enter(@Advice.Argument(0) Object request, @Advice.Origin Class<?> endpoint) throws Exception {
        return begin(request, endpoint);
    }

    @Advice.OnMethodExit(onThrowable = Throwable.class, suppress = Throwable.class)
    public static void exit(@Advice.Enter boolean active, @Advice.Thrown Throwable thrown) throws Exception {
        finish(active, thrown);
    }

    public static boolean begin(Object request, Class<?> endpoint) throws Exception {
        Object active = call("servletEnter", new Class<?>[] { Object.class }, request);
        if (!Boolean.TRUE.equals(active)) return false;
        call("endpointEnter", new Class<?>[] { Class.class }, endpoint);
        return true;
    }

    public static void finish(boolean active, Throwable thrown) throws Exception {
        if (!active) return;
        call("endpointExit", new Class<?>[] { Throwable.class }, thrown);
        call("servletExit", new Class<?>[] { boolean.class }, true);
    }

    static Object call(String name, Class<?>[] signature, Object... arguments) throws Exception {
        Class<?> bridge = Class.forName("io.legacylens.agent.bridge.AgentBridge", true, null);
        return bridge.getMethod(name, signature).invoke(null, arguments);
    }
}
