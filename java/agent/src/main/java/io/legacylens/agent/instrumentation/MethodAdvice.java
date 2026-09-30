package io.legacylens.agent.instrumentation;
import io.legacylens.agent.bridge.AgentBridge;
import net.bytebuddy.asm.Advice;
public final class MethodAdvice {
 @Advice.OnMethodEnter public static boolean enter(@Advice.Origin("#t") String type,@Advice.Origin("#m") String method,@Advice.Origin("#d") String descriptor,@Advice.Origin Class<?> owner){return AgentBridge.methodEnter(type,method,descriptor,owner);}
 @Advice.OnMethodExit(onThrowable=Throwable.class) public static void exit(@Advice.Origin("#t") String type,@Advice.Origin("#m") String method,@Advice.Origin("#d") String descriptor,@Advice.Origin Class<?> owner,@Advice.Thrown Throwable thrown,@Advice.Enter boolean root){AgentBridge.methodExit(type,method,descriptor,owner,thrown,root);}
}
