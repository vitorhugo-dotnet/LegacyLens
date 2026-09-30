package io.legacylens.agent;

import net.bytebuddy.agent.ByteBuddyAgent;
import org.junit.jupiter.api.Test;
import java.lang.reflect.Method;
import java.io.InputStream;
import static org.junit.jupiter.api.Assertions.*;
import sample.app.SampleApplication;
import javax.servlet.fake.FakeServlet;

class AgentFlowTest {
    @Test void adviceTransformsLoadedApplicationMethodAndCapturesExceptionWithoutMessage() throws Exception {
        AgentTransport transport = AgentTransport.forTesting(16);
        LegacyLensAgent.installForTesting(ByteBuddyAgent.install(), transport, AgentConfig.forTesting("sample.app"));
        String secret = "db password do-not-record";
        try { new FakeServlet().service(new FakeServlet.Request(secret),new Object()); fail("expected application error"); }
        catch (IllegalStateException expected) { assertEquals(secret, expected.getMessage()); }
        java.util.List<String> kinds=new java.util.ArrayList<String>(); java.util.List<Event> observed=new java.util.ArrayList<Event>(); Event terminal=null; for(Event e;(e=transport.pollForTesting())!=null;){observed.add(e);kinds.add(e.kind);if(e.kind.equals("method.error"))terminal=e;}
        assertTrue(kinds.contains("http.server"),kinds.toString());
        assertTrue(kinds.contains("db.query"),kinds.toString());
        assertEquals(1,kinds.stream().filter("db.query"::equals).count(),"nested driver wrapper must not duplicate JDBC effect");
        assertNotNull(terminal); assertTrue(terminal.metadata.containsKey("code.method"));
        Event jdbc=observed.stream().filter(e->e.kind.equals("db.query")).findFirst().get();
        assertEquals("SELECT * FROM orders WHERE id = ?",jdbc.metadata.get("sql"));
        assertEquals("GET",observed.stream().filter(e->e.kind.equals("http.server")).findFirst().get().metadata.get("http.method"));
        long expectedSequence=1;for(Event event:observed){assertEquals("p1",event.projectId);assertEquals("0123456789abcdef0123456789abcdef",event.traceId);assertEquals(expectedSequence++,event.sequence);assertTrue(event.occurredAt.endsWith("Z"));assertFalse(event.toJson().contains(secret));}
        assertTrue(terminal.metadata.containsKey("code.line") || terminal.metadata.containsKey("code.line_missing"));
    }
    @Test void uncorrelatedServletDoesNotCreateAnOrphanTrace() {
        assertNull(TraceContext.current());
        boolean active=LegacyLensAgent.servletEnter(new FakeServlet.Request(){public String getHeader(String name){return "invalid";}});
        assertFalse(active);assertNull(TraceContext.current());
    }
    @Test void jakartaServletImplementationIsInstrumentedSeparately() {
        AgentTransport transport=AgentTransport.forTesting(16);
        LegacyLensAgent.installForTesting(ByteBuddyAgent.install(),transport,AgentConfig.forTesting("sample.app"));
        new jakarta.servlet.fake.FakeServlet().service(new jakarta.servlet.fake.FakeServlet.Request(),new Object());
        Event http=null;for(Event event;(event=transport.pollForTesting())!=null;)if(event.kind.equals("http.server"))http=event;
        assertNotNull(http);assertEquals("POST",http.metadata.get("http.method"));
        assertFalse(http.toJson().contains("/private/not-recorded"));
    }
    @Test void equalNamesInDifferentLoadersRetainDeploymentIdentity() throws Exception {
        Class<?> first=isolatedSampleClass(); Class<?> second=isolatedSampleClass();
        assertNotSame(first,second); assertEquals(first.getName(),second.getName());
        String firstIdentity=AgentConfig.deploymentIdentity("deployment",first.getClassLoader());
        String secondIdentity=AgentConfig.deploymentIdentity("deployment",second.getClassLoader());
        assertNotEquals(firstIdentity,secondIdentity);
        AgentTransport transport=AgentTransport.forTesting(16); LegacyLensAgent.installForTesting(ByteBuddyAgent.install(),transport,AgentConfig.forTesting("sample.app"));
        TraceContext.begin("0123456789abcdef0123456789abcdef",null);
        first.getMethod("load",String.class).invoke(first.getDeclaredConstructor().newInstance(),new Object[]{null});
        second.getMethod("load",String.class).invoke(second.getDeclaredConstructor().newInstance(),new Object[]{null});
        TraceContext.end();
        Event a=null,b=null; for(Event e;(e=transport.pollForTesting())!=null;){if(e.kind.equals("method.start")){if(a==null)a=e;else b=e;}} assertNotNull(a);assertNotNull(b);
        assertNotEquals(a.metadata.get("code.deployment"),b.metadata.get("code.deployment"));
    }
    private Class<?> isolatedSampleClass() throws Exception {byte[] bytes;try(InputStream in=getClass().getResourceAsStream("/sample/app/SampleApplication.class")){bytes=new byte[in.available()];int n=in.read(bytes);assertEquals(bytes.length,n);}return new ClassLoader(getClass().getClassLoader()){protected Class<?> loadClass(String name,boolean resolve)throws ClassNotFoundException{synchronized(getClassLoadingLock(name)){if(name.equals("sample.app.SampleApplication")){Class<?> c=findLoadedClass(name);if(c==null)c=defineClass(name,bytes,0,bytes.length);if(resolve)resolveClass(c);return c;}return super.loadClass(name,resolve);}}}.loadClass("sample.app.SampleApplication");}
}
