package io.legacylens.agent.instrumentation;
import io.legacylens.agent.bridge.AgentBridge;
import net.bytebuddy.asm.Advice;
public final class ServletAdvice {
 @Advice.OnMethodEnter public static boolean enter(@Advice.Argument(0) Object request){return AgentBridge.servletEnter(request);}
 @Advice.OnMethodExit(onThrowable=Throwable.class) public static void exit(@Advice.Enter boolean active){AgentBridge.servletExit(active);}
}
