package io.legacylens.agent.instrumentation;
import net.bytebuddy.asm.Advice;
public final class ServletAdvice {
 @Advice.OnMethodEnter public static boolean enter(@Advice.Argument(0) Object request){
  try { Class<?> bridge=Class.forName("io.legacylens.agent.bridge.AgentBridge",true,null);return Boolean.TRUE.equals(bridge.getMethod("servletEnter",Object.class).invoke(null,request)); }
  catch(Throwable ignored){return false;}
 }
 @Advice.OnMethodExit(onThrowable=Throwable.class) public static void exit(@Advice.Enter boolean active){
  if(!active)return;
  try { Class<?> bridge=Class.forName("io.legacylens.agent.bridge.AgentBridge",true,null);bridge.getMethod("servletExit",boolean.class).invoke(null,active); }
  catch(Throwable ignored){}
 }
}
