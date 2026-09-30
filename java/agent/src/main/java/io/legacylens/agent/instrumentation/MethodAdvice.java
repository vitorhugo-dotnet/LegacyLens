package io.legacylens.agent.instrumentation;
import io.legacylens.agent.LegacyLensAgent;
import net.bytebuddy.asm.Advice;
public final class MethodAdvice {
 @Advice.OnMethodEnter public static boolean enter(@Advice.Origin("#t") String type,@Advice.Origin("#m") String method,@Advice.Origin("#d") String descriptor,@Advice.Origin Class<?> owner){return LegacyLensAgent.methodEnter(type,method,descriptor,owner.getClassLoader());}
 @Advice.OnMethodExit(onThrowable=Throwable.class) public static void exit(@Advice.Origin("#t") String type,@Advice.Origin("#m") String method,@Advice.Origin("#d") String descriptor,@Advice.Origin Class<?> owner,@Advice.Thrown Throwable thrown,@Advice.Enter boolean root){LegacyLensAgent.methodExit(type,method,descriptor,owner.getClassLoader(),thrown,root);}
}
