package io.legacylens.agent.instrumentation;
import io.legacylens.agent.LegacyLensAgent;
import net.bytebuddy.asm.Advice;
public final class JdbcAdvice {
 @Advice.OnMethodEnter public static boolean enter(@Advice.Origin("#m") String method,@Advice.Argument(0) Object arg){String sql=arg instanceof String?(String)arg:null;return LegacyLensAgent.jdbcEnter(method.startsWith("prepare")?"prepare":method.startsWith("executeQuery")?"query":"execute",sql);}
 @Advice.OnMethodExit(onThrowable=Throwable.class) public static void exit(@Advice.Origin("#m") String method,@Advice.Argument(0) Object arg,@Advice.Enter boolean outer){String sql=arg instanceof String?(String)arg:null;LegacyLensAgent.jdbcExit(method.startsWith("prepare")?"prepare":method.startsWith("executeQuery")?"query":"execute",sql,outer);}
}
