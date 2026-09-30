package io.legacylens.agent;

import io.legacylens.agent.instrumentation.JdbcAdvice;
import io.legacylens.agent.instrumentation.MethodAdvice;
import io.legacylens.agent.instrumentation.ServletAdvice;
import net.bytebuddy.agent.builder.AgentBuilder;
import net.bytebuddy.asm.Advice;
import net.bytebuddy.matcher.ElementMatchers;
import java.io.IOException;
import java.lang.instrument.Instrumentation;
import java.util.LinkedHashMap;
import java.util.Map;
import static net.bytebuddy.matcher.ElementMatchers.*;

public final class LegacyLensAgent {
    private static volatile AgentConfig config; private static volatile AgentTransport transport; private static volatile boolean installed;
    private static final ThreadLocal<Integer> jdbcDepth=new ThreadLocal<Integer>();
    private LegacyLensAgent(){}
    public static void premain(String options,Instrumentation instrumentation){try{config=AgentConfig.load(options);transport=AgentTransport.start(config);install(instrumentation,config);}catch(Exception e){System.err.println("LegacyLens agent configuration failed: "+e.getClass().getSimpleName());}}
    static synchronized void installForTesting(Instrumentation i,AgentTransport t,AgentConfig c){config=c;transport=t;if(!installed){install(i,c);installed=true;}}
    private static void install(Instrumentation i,final AgentConfig c){
        new AgentBuilder.Default().ignore(nameStartsWith("io.legacylens.").or(nameStartsWith("net.bytebuddy.")).or(nameStartsWith("java.")))
          .type(hasSuperType(named("javax.servlet.Servlet")).or(hasSuperType(named("jakarta.servlet.Servlet"))))
          .transform((b,td,cl,m,p)->b.visit(Advice.to(ServletAdvice.class).on(named("service").and(takesArguments(2)))))
          .type(nameStartsWith("java.sql.").or(nameStartsWith("com.mysql.")).or(nameStartsWith("org.mariadb.")))
          .transform((b,td,cl,m,p)->b.visit(Advice.to(JdbcAdvice.class).on(namedOneOf("execute","executeQuery","executeUpdate","prepareStatement","prepareCall").and(takesArguments(1)))))
          .type(new net.bytebuddy.matcher.ElementMatcher<net.bytebuddy.description.type.TypeDescription>(){public boolean matches(net.bytebuddy.description.type.TypeDescription t){String n=t.getName();if(n.startsWith("io.legacylens.")||n.startsWith("java.")||n.startsWith("javax.")||n.startsWith("jakarta.")||n.startsWith("net.bytebuddy."))return false;for(String a:c.packages)if(n.equals(a)||n.startsWith(a+"."))return true;return false;}})
          .transform((b,td,cl,m,p)->b.visit(Advice.to(MethodAdvice.class).on(isMethod().and(not(isAbstract())).and(not(isNative())))))
          .with(AgentBuilder.RedefinitionStrategy.RETRANSFORMATION).installOn(i);
    }
    public static boolean servletEnter(Object request){String parent=null,trace=null;try{String tp=(String)request.getClass().getMethod("getHeader",String.class).invoke(request,"traceparent");if(TraceContext.validTraceparent(tp)){trace=tp.substring(3,35);parent=tp.substring(36,52);}}catch(Exception ignored){}if(trace==null)return false;TraceContext.begin(trace,parent);Map<String,String> m=new LinkedHashMap<String,String>();try{String method=String.valueOf(request.getClass().getMethod("getMethod").invoke(request));if(method.matches("[A-Z]{1,12}"))m.put("http.method",method);}catch(Exception ignored){}emit("http.server",m);return true;}
    public static void servletExit(boolean active){if(!active)return;emit("http.server.end",new LinkedHashMap<String,String>());TraceContext.end();}
    public static boolean methodEnter(String type,String name,String desc,ClassLoader loader){if(TraceContext.current()==null)return false;Map<String,String>m=identity(type,name,desc,loader);emit("method.start",m);return true;}
    public static void methodExit(String type,String name,String desc,ClassLoader loader,Throwable thrown,boolean active){if(!active)return;Map<String,String>m=identity(type,name,desc,loader);if(thrown==null)emit("method.end",m);else emit("method.error",m);}
    static Map<String,String> identity(String type,String name,String desc,ClassLoader loader){Map<String,String>m=new LinkedHashMap<String,String>();m.put("code.class",type);m.put("code.method",name);m.put("code.descriptor",desc);m.put("code.deployment",AgentConfig.deploymentIdentity("deployment",loader));m.put("code.line_missing","true");return m;}
    public static boolean jdbcEnter(String operation,String sql){Integer depth=jdbcDepth.get();int current=depth==null?0:depth;jdbcDepth.set(current+1);if(current>0)return false;String safe=Sanitizer.sql(sql);Map<String,String>m=new LinkedHashMap<String,String>();m.put("db.operation",operation);m.put("db.system","jdbc");if(!safe.isEmpty())m.put("sql",safe);emit("db."+operation,m);return true;}
    public static void jdbcExit(String operation,String sql,boolean outer){Integer depth=jdbcDepth.get();if(depth==null)return;if(depth<=1)jdbcDepth.remove();else jdbcDepth.set(depth-1);}
    private static void emit(String kind,Map<String,String>m){AgentConfig c=config;AgentTransport t=transport;TraceContext trace=TraceContext.current();if(c==null||t==null||trace==null)return;Event e=new Event(c.projectId,trace.traceId,c.producerId,t.nextSequence(trace.traceId),kind,c.revision,m);t.offer(e);}
}
