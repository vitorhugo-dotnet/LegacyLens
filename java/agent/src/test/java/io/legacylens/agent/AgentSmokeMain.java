package io.legacylens.agent;
import javax.servlet.fake.FakeServlet;
import java.lang.reflect.Field;
import java.util.Map;
/** Small entry point used only to smoke-test the shaded -javaagent artifact. */
public final class AgentSmokeMain {
 public static void main(String[] args){
  try {
   new FakeServlet().service(new FakeServlet.Request(null),new Object());
   Class<?> bridge=Class.forName("io.legacylens.agent.bridge.AgentBridge",true,null);
   if(bridge.getClassLoader()!=null)throw new AssertionError("bridge was not defined by the bootstrap loader");
   Class<?> agent=Class.forName("io.legacylens.agent.LegacyLensAgent");
   Field transportField=agent.getDeclaredField("transport");transportField.setAccessible(true);
   Object transport=transportField.get(null);
   Field lockField=transport.getClass().getDeclaredField("stateLock");lockField.setAccessible(true);
   Field statesField=transport.getClass().getDeclaredField("traceStates");statesField.setAccessible(true);
   String traceId="0123456789abcdef0123456789abcdef";
   int attempts=0;
   while(attempts++<100){
    synchronized(lockField.get(transport)){
     Map<?,?> states=(Map<?,?>)statesField.get(transport);Object state=states.get(traceId);
     if(state!=null){Field sequence=state.getClass().getDeclaredField("sequence");sequence.setAccessible(true);if(sequence.getLong(state)>=4)return;}
    }
    Thread.sleep(10L);
   }
   throw new AssertionError("javaagent did not emit HTTP, application-method, and JDBC events through the bootstrap bridge");
  }catch(Throwable failure){throw new AssertionError("agent smoke application did not prove transformed event flow",failure);}
 }
}
