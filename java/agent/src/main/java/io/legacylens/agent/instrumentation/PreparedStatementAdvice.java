package io.legacylens.agent.instrumentation;

import net.bytebuddy.asm.Advice;

public final class PreparedStatementAdvice {
    @Advice.OnMethodEnter public static boolean enter(@Advice.This Object statement, @Advice.Origin("#m") String method) {
        try { Class<?> bridge=Class.forName("io.legacylens.agent.bridge.AgentBridge",true,null);return Boolean.TRUE.equals(bridge.getMethod("preparedExecute",Object.class,String.class).invoke(null,statement,method)); }
        catch(Throwable ignored){return false;}
    }

    @Advice.OnMethodExit(onThrowable = Throwable.class) public static void exit(@Advice.Origin("#m") String method, @Advice.Enter boolean outer) {
        try { Class<?> bridge=Class.forName("io.legacylens.agent.bridge.AgentBridge",true,null);bridge.getMethod("jdbcExit",String.class,String.class,boolean.class).invoke(null,method,null,outer); }
        catch(Throwable ignored){}
    }
}
