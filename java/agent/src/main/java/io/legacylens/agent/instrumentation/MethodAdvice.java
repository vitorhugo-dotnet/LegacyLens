package io.legacylens.agent.instrumentation;

import io.legacylens.agent.bridge.AgentBridge;
import net.bytebuddy.asm.Advice;

public final class MethodAdvice {
    @Advice.OnMethodEnter
    public static int enter(@Advice.Origin("#t") String type,
                            @Advice.Origin("#m") String method,
                            @Advice.Origin("#d") String descriptor,
                            @Advice.Origin Class<?> owner) {
        int state = 0;
        try {
            if (AgentBridge.facesActionEnter(owner, method)) state |= 2;
        } catch (Throwable ignored) { }
        try {
            if (AgentBridge.methodEnter(type, method, descriptor, owner)) state |= 1;
        } catch (Throwable ignored) { }
        return state;
    }

    @Advice.OnMethodExit(onThrowable = Throwable.class)
    public static void exit(@Advice.Origin("#t") String type,
                            @Advice.Origin("#m") String method,
                            @Advice.Origin("#d") String descriptor,
                            @Advice.Origin Class<?> owner,
                            @Advice.Thrown Throwable thrown,
                            @Advice.Enter int state) {
        if (state == 0) return;
        if ((state & 1) != 0) {
            try { AgentBridge.methodExit(type, method, descriptor, owner, thrown, true); }
            catch (Throwable ignored) { }
        }
        if ((state & 2) != 0) {
            try { AgentBridge.facesActionExit(owner, method, thrown); }
            catch (Throwable ignored) { }
        }
    }
}
