package io.legacylens.agent.instrumentation;
import io.legacylens.agent.LegacyLensAgent;
import net.bytebuddy.asm.Advice;
public final class ServletAdvice {
 @Advice.OnMethodEnter public static boolean enter(@Advice.Argument(0) Object request){return LegacyLensAgent.servletEnter(request);}
 @Advice.OnMethodExit(onThrowable=Throwable.class) public static void exit(@Advice.Enter boolean active){LegacyLensAgent.servletExit(active);}
}
