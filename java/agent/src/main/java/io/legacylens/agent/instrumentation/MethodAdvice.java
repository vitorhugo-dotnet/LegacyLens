package io.legacylens.agent.instrumentation;
import net.bytebuddy.asm.Advice;
public final class MethodAdvice {
 @Advice.OnMethodEnter public static int enter(@Advice.Origin("#t") String type,@Advice.Origin("#m") String method,@Advice.Origin("#d") String descriptor,@Advice.Origin Class<?> owner){
  Class<?> bridge;
  try { bridge=Class.forName("io.legacylens.agent.bridge.AgentBridge",true,null); }
  catch(Throwable ignored){return 0;}
  int state=0;
  try { if(Boolean.TRUE.equals(bridge.getMethod("facesActionEnter",Class.class,String.class).invoke(null,owner,method)))state|=2; }
  catch(Throwable ignored){}
  try { if(Boolean.TRUE.equals(bridge.getMethod("methodEnter",String.class,String.class,String.class,Class.class).invoke(null,type,method,descriptor,owner)))state|=1; }
  catch(Throwable ignored){}
  return state;
 }
 @Advice.OnMethodExit(onThrowable=Throwable.class) public static void exit(@Advice.Origin("#t") String type,@Advice.Origin("#m") String method,@Advice.Origin("#d") String descriptor,@Advice.Origin Class<?> owner,@Advice.Thrown Throwable thrown,@Advice.Enter int state){
  if(state==0)return;
  try {
   Class<?> bridge=Class.forName("io.legacylens.agent.bridge.AgentBridge",true,null);
   if((state&1)!=0)bridge.getMethod("methodExit",String.class,String.class,String.class,Class.class,Throwable.class,boolean.class).invoke(null,type,method,descriptor,owner,thrown,true);
   if((state&2)!=0)bridge.getMethod("facesActionExit",Class.class,String.class,Throwable.class).invoke(null,owner,method,thrown);
  }
  catch(Throwable ignored){}
 }
}
