package io.legacylens.agent.instrumentation;
import net.bytebuddy.asm.Advice;
public final class ServletAdvice {
 @Advice.OnMethodEnter(suppress=Throwable.class) public static boolean enter(@Advice.Argument(0) Object request,@Advice.Origin Class<?> endpoint){
  try {
   Class<?> bridge=Class.forName("io.legacylens.agent.bridge.AgentBridge",true,null);
   boolean active=Boolean.TRUE.equals(bridge.getMethod("servletEnter",Object.class).invoke(null,request));
   if(active)bridge.getMethod("endpointEnter",Class.class).invoke(null,endpoint);
   return active;
  }
  catch(Throwable ignored){return false;}
 }
 @Advice.OnMethodExit(onThrowable=Throwable.class,suppress=Throwable.class) public static void exit(@Advice.Enter boolean active,@Advice.Thrown Throwable thrown){
  if(!active)return;
  try {
   Class<?> bridge=Class.forName("io.legacylens.agent.bridge.AgentBridge",true,null);
   bridge.getMethod("endpointExit",Throwable.class).invoke(null,thrown);
   bridge.getMethod("servletExit",boolean.class).invoke(null,true);
  }
  catch(Throwable ignored){}
 }
}
