package io.legacylens.agent.instrumentation;

import net.bytebuddy.asm.Advice;

public final class PrepareStatementAdvice {
    @Advice.OnMethodExit(onThrowable = Throwable.class)
    public static void exit(@Advice.Argument(0) String sql,
                            @Advice.Return Object statement,
                            @Advice.Thrown Throwable thrown) {
        if (thrown == null) try { Class<?> bridge=Class.forName("io.legacylens.agent.bridge.AgentBridge",true,null);bridge.getMethod("preparedStatement",Object.class,String.class).invoke(null,statement,sql); }
        catch(Throwable ignored){}
    }
}
