package io.legacylens.agent.instrumentation;

import io.legacylens.agent.bridge.AgentBridge;
import net.bytebuddy.asm.Advice;

public final class PrepareStatementAdvice {
    @Advice.OnMethodExit(onThrowable = Throwable.class)
    public static void exit(@Advice.Argument(0) String sql,
                            @Advice.Return Object statement,
                            @Advice.Thrown Throwable thrown) {
        if (thrown == null) AgentBridge.preparedStatement(statement, sql);
    }
}
