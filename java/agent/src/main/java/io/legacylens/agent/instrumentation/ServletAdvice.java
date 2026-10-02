package io.legacylens.agent.instrumentation;
import net.bytebuddy.asm.Advice;
public final class ServletAdvice {
 @Advice.OnMethodEnter(suppress=Throwable.class) public static boolean enter(@Advice.Argument(0) Object request,@Advice.Origin Class<?> endpoint){
  try { return EndpointAdvice.begin(request,endpoint); }
  catch(Throwable ignored){return false;}
 }
 @Advice.OnMethodExit(onThrowable=Throwable.class,suppress=Throwable.class) public static void exit(@Advice.Enter boolean active,@Advice.Thrown Throwable thrown){
  if(!active)return;
  try { EndpointAdvice.finish(active,thrown); }
  catch(Throwable ignored){}
 }
}
