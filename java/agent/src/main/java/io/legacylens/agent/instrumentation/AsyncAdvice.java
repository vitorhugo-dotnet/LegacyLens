package io.legacylens.agent.instrumentation;

import io.legacylens.agent.bridge.AgentBridge;
import net.bytebuddy.asm.Advice;

/** Captures request context for tasks submitted to supported executors. */
public final class AsyncAdvice {
    private AsyncAdvice() { }
    @Advice.OnMethodEnter
    public static void onEnter(@Advice.Argument(value = 0, readOnly = false) Runnable task) {
        if (task != null) task = AgentBridge.wrapRunnable(task);
    }

    @Advice.OnMethodExit(onThrowable = Throwable.class)
    public static void onExit(@Advice.Argument(0) Runnable task, @Advice.Thrown Throwable thrown) {
        if (thrown != null && task != null) AgentBridge.discardRunnable(task);
    }
}
