package io.legacylens.agent.instrumentation;
import net.bytebuddy.asm.Advice;
public final class JdbcAdvice {
 @Advice.OnMethodEnter public static boolean enter(@Advice.Origin("#m") String method,@Advice.Argument(0) Object arg){String sql=arg instanceof String?(String)arg:null;String operation=method.equals("executeQuery")?"query":method.equals("executeUpdate")?"update":"execute";
  try { Class<?> bridge=Class.forName("io.legacylens.agent.bridge.AgentBridge",true,null);return Boolean.TRUE.equals(bridge.getMethod("jdbcEnter",String.class,String.class).invoke(null,operation,sql)); }
  catch(Throwable ignored){return false;}
 }
 @Advice.OnMethodExit(onThrowable=Throwable.class) public static void exit(@Advice.Origin("#m") String method,@Advice.Argument(0) Object arg,@Advice.Enter boolean outer){String sql=arg instanceof String?(String)arg:null;String operation=method.equals("executeQuery")?"query":method.equals("executeUpdate")?"update":"execute";
  try { Class<?> bridge=Class.forName("io.legacylens.agent.bridge.AgentBridge",true,null);bridge.getMethod("jdbcExit",String.class,String.class,boolean.class).invoke(null,operation,sql,outer); }
  catch(Throwable ignored){}
 }
}
