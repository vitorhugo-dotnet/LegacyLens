package io.legacylens.agent.instrumentation;

import io.legacylens.agent.bridge.AgentBridge;
import net.bytebuddy.asm.Advice;

public final class PreparedStatementAdvice {
    @Advice.OnMethodEnter public static boolean enter(@Advice.This Object statement, @Advice.Origin("#m") String method) {
        return AgentBridge.preparedExecute(statement, method);
    }

    @Advice.OnMethodExit(onThrowable = Throwable.class) public static void exit(@Advice.Origin("#m") String method, @Advice.Enter boolean outer) {
        AgentBridge.jdbcExit(method, null, outer);
    }
}
