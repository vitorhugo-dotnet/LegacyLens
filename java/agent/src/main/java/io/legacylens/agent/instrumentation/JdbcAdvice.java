package io.legacylens.agent.instrumentation;
import io.legacylens.agent.bridge.AgentBridge;
import net.bytebuddy.asm.Advice;
public final class JdbcAdvice {
 @Advice.OnMethodEnter public static boolean enter(@Advice.Origin("#m") String method,@Advice.Argument(0) Object arg){String sql=arg instanceof String?(String)arg:null;String operation=method.equals("executeQuery")?"query":method.equals("executeUpdate")?"update":"execute";return AgentBridge.jdbcEnter(operation,sql);}
 @Advice.OnMethodExit(onThrowable=Throwable.class) public static void exit(@Advice.Origin("#m") String method,@Advice.Argument(0) Object arg,@Advice.Enter boolean outer){String sql=arg instanceof String?(String)arg:null;String operation=method.equals("executeQuery")?"query":method.equals("executeUpdate")?"update":"execute";AgentBridge.jdbcExit(operation,sql,outer);}
}
