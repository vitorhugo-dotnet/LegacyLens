package io.legacylens.agent.instrumentation;
import net.bytebuddy.asm.Advice;
public final class MethodAdvice {
 @Advice.OnMethodEnter public static boolean enter(@Advice.Origin("#t") String type,@Advice.Origin("#m") String method,@Advice.Origin("#d") String descriptor,@Advice.Origin Class<?> owner){
  try { Class<?> bridge=Class.forName("io.legacylens.agent.bridge.AgentBridge",true,null);return Boolean.TRUE.equals(bridge.getMethod("methodEnter",String.class,String.class,String.class,Class.class).invoke(null,type,method,descriptor,owner)); }
  catch(Throwable ignored){return false;}
 }
 @Advice.OnMethodExit(onThrowable=Throwable.class) public static void exit(@Advice.Origin("#t") String type,@Advice.Origin("#m") String method,@Advice.Origin("#d") String descriptor,@Advice.Origin Class<?> owner,@Advice.Thrown Throwable thrown,@Advice.Enter boolean root){
  if(!root)return;
  try { Class<?> bridge=Class.forName("io.legacylens.agent.bridge.AgentBridge",true,null);bridge.getMethod("methodExit",String.class,String.class,String.class,Class.class,Throwable.class,boolean.class).invoke(null,type,method,descriptor,owner,thrown,root); }
  catch(Throwable ignored){}
 }
}
