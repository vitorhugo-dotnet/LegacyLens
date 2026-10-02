package io.legacylens.agent;

import net.bytebuddy.agent.ByteBuddyAgent;
import org.junit.jupiter.api.Test;
import java.lang.reflect.Method;
import java.io.InputStream;
import net.bytebuddy.ByteBuddy;
import net.bytebuddy.implementation.StubMethod;
import static org.junit.jupiter.api.Assertions.*;
import sample.app.SampleApplication;
import javax.servlet.fake.FakeServlet;

class AgentFlowTest {
    @Test void configuredLegacyLensPackageIsEligibleForMethodInstrumentation() {
        java.util.Set<String> packages = java.util.Collections.singleton("io.legacylens.fixture");
        assertTrue(LegacyLensAgent.instrumentableApplicationType("io.legacylens.fixture.OrderService", packages));
        assertFalse(LegacyLensAgent.instrumentableApplicationType("io.legacylens.agent.LegacyLensAgent", packages));
        assertFalse(LegacyLensAgent.instrumentableApplicationType("io.legacylens.fixtureother.OrderService", packages));
    }
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
        Event http=observed.stream().filter(e->e.kind.equals("http.server")).findFirst().get();
        Event methodStart=observed.stream().filter(e->e.kind.equals("method.start")&&"load".equals(e.metadata.get("code.method"))).findFirst().get();
        assertEquals("GET",http.metadata.get("http.method"));
        assertEquals("0123456789abcdef",http.metadata.get("http.request_span"));
        assertNull(http.parentEventId);
        assertEquals(http.eventId,methodStart.parentEventId,"application method must be causally nested in HTTP request");
        assertEquals(methodStart.eventId,jdbc.parentEventId,"JDBC call must be causally nested in application method");
        assertEquals(methodStart.eventId,terminal.parentEventId,"exception event must unwind against the matching method start");
        assertTrue(jdbc.toJson().contains("\"parentEventId\":\""+methodStart.eventId+"\""));
        long expectedSequence=1;for(Event event:observed){assertEquals("p1",event.projectId);assertEquals("0123456789abcdef0123456789abcdef",event.traceId);assertEquals(expectedSequence++,event.sequence);assertTrue(event.occurredAt.endsWith("Z"));assertFalse(event.toJson().contains(secret));}
        assertEquals("5",terminal.metadata.get("code.line"),"debug line table must be captured when present");
        assertFalse(terminal.metadata.containsKey("code.line_missing"));
    }
    @Test void generatedTraceAndSpanIdsHaveW3cShape() {
        for(int i=0;i<32;i++){
            String trace=TraceContext.hex(16),span=TraceContext.hex(8);
            assertTrue(trace.matches("[0-9a-f]{32}")&&!trace.matches("0{32}"));
            assertTrue(span.matches("[0-9a-f]{16}")&&!span.matches("0{16}"));
        }
    }
    @Test void noDebugLineHasExplicitAbsence() throws Exception {
        AgentTransport transport=AgentTransport.forTesting(16);LegacyLensAgent.installForTesting(ByteBuddyAgent.install(),transport,AgentConfig.forTesting("sample.app"));
        LegacyLensAgent.beginTraceForTesting("0123456789abcdef0123456789abcdef");
        try { Class<?> type=isolatedClassWithoutDebugLine(); type.getMethod("run").invoke(type.getDeclaredConstructor().newInstance()); }
        finally { LegacyLensAgent.endTraceForTesting(); }
        Event start=null;for(Event event;(event=transport.pollForTesting())!=null;)if(event.kind.equals("method.start")&&"run".equals(event.metadata.get("code.method")))start=event;
        assertNotNull(start);assertEquals("true",start.metadata.get("code.line_missing"));assertFalse(start.metadata.containsKey("code.line"));
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
    @Test void servletAdviceUsesBootstrapBridgeFromIsolatedDeploymentLoader() throws Exception {
        AgentTransport transport=AgentTransport.forTesting(16);
        LegacyLensAgent.installForTesting(ByteBuddyAgent.install(),transport,AgentConfig.forTesting("sample.app"));
        final java.util.Map<String,byte[]> deploymentClasses=new java.util.HashMap<String,byte[]>();
        deploymentClasses.put("javax.servlet.fake.FakeServlet",classBytes("/javax/servlet/fake/FakeServlet.class"));
        deploymentClasses.put("javax.servlet.fake.FakeServlet$Request",classBytes("/javax/servlet/fake/FakeServlet$Request.class"));
        ClassLoader deployment=new ClassLoader(getClass().getClassLoader()) {
            @Override protected Class<?> loadClass(String name,boolean resolve)throws ClassNotFoundException {
                if(name.startsWith("io.legacylens.agent."))throw new ClassNotFoundException(name);
                if(deploymentClasses.containsKey(name))synchronized(getClassLoadingLock(name)) {
                    Class<?> type=findLoadedClass(name);byte[] bytes=deploymentClasses.get(name);if(type==null)type=defineClass(name,bytes,0,bytes.length);
                    if(resolve)resolveClass(type);return type;
                }
                return super.loadClass(name,resolve);
            }
        };
        Class<?> servletType=deployment.loadClass("javax.servlet.fake.FakeServlet");
        Object servlet=servletType.getDeclaredConstructor().newInstance();
        Object request=deployment.loadClass("javax.servlet.fake.FakeServlet$Request").getDeclaredConstructor().newInstance();
        servletType.getMethod("service",Object.class,Object.class).invoke(servlet,request,new Object());
        java.util.List<String> kinds=new java.util.ArrayList<String>();
        for(Event event;(event=transport.pollForTesting())!=null;)kinds.add(event.kind);
        assertTrue(kinds.contains("http.server"),kinds.toString());
        assertTrue(kinds.contains("endpoint.start"),kinds.toString());
    }
    @Test void equalNamesInDifferentLoadersRetainDeploymentIdentity() throws Exception {
        Class<?> first=isolatedSampleClass(); Class<?> second=isolatedSampleClass();
        assertNotSame(first,second); assertEquals(first.getName(),second.getName());
        String firstIdentity=AgentConfig.deploymentIdentity("deployment",first.getClassLoader());
        String secondIdentity=AgentConfig.deploymentIdentity("deployment",second.getClassLoader());
        assertNotEquals(firstIdentity,secondIdentity);
        AgentTransport transport=AgentTransport.forTesting(16); LegacyLensAgent.installForTesting(ByteBuddyAgent.install(),transport,AgentConfig.forTesting("sample.app"));
        LegacyLensAgent.beginTraceForTesting("0123456789abcdef0123456789abcdef");
        try {
            first.getMethod("load",String.class).invoke(first.getDeclaredConstructor().newInstance(),new Object[]{null});
            second.getMethod("load",String.class).invoke(second.getDeclaredConstructor().newInstance(),new Object[]{null});
        } finally { LegacyLensAgent.endTraceForTesting(); }
        Event a=null,b=null; for(Event e;(e=transport.pollForTesting())!=null;){if(e.kind.equals("method.start")){if(a==null)a=e;else b=e;}} assertNotNull(a);assertNotNull(b);
        assertNotEquals(a.metadata.get("code.deployment"),b.metadata.get("code.deployment"));
    }
    @Test void isolatedNonDelegatingLoaderCanResolveAgentBridge() throws Exception {
        AgentTransport transport=AgentTransport.forTesting(16);LegacyLensAgent.installForTesting(ByteBuddyAgent.install(),transport,AgentConfig.forTesting("sample.app"));
        LegacyLensAgent.beginTraceForTesting("0123456789abcdef0123456789abcdef");
        try { Class<?> type=isolatedSampleClassWithoutParent(); type.getMethod("run").invoke(type.getDeclaredConstructor().newInstance()); }
        finally { LegacyLensAgent.endTraceForTesting(); }
        Event observed=null;for(Event event;(event=transport.pollForTesting())!=null;)if(event.kind.equals("method.start")&&"run".equals(event.metadata.get("code.method")))observed=event;
        assertNotNull(observed,"isolated class loader must reach the bootstrap bridge");
    }
    @Test void preparedStatementExecutionCarriesOnlyPreparedSqlIdentity() throws Exception {
        AgentTransport transport=AgentTransport.forTesting(16);LegacyLensAgent.installForTesting(ByteBuddyAgent.install(),transport,AgentConfig.forTesting("sample.app"));
        LegacyLensAgent.beginTraceForTesting("0123456789abcdef0123456789abcdef");
        try { new SampleApplication().loadPrepared(); }
        finally { LegacyLensAgent.endTraceForTesting(); }
        java.util.List<Event> events=new java.util.ArrayList<Event>();for(Event event;(event=transport.pollForTesting())!=null;)events.add(event);
        java.util.List<Event> queries=new java.util.ArrayList<Event>();for(Event event:events)if(event.kind.equals("db.query"))queries.add(event);
        assertEquals(1,queries.size(),"prepared wrapper/driver execution is one database effect");
        assertEquals("SELECT * FROM orders WHERE id = ?",queries.get(0).metadata.get("sql"));
        assertFalse(queries.get(0).toJson().contains("parameter-secret"));
        assertTrue(events.stream().noneMatch(event->event.kind.equals("db.prepare")),"preparation alone is not an executed query");
    }
    @Test void nestedServletsKeepCausalParentsInsideTheirTrace() throws Exception {
        AgentTransport transport=AgentTransport.forTesting(64);
        LegacyLensAgent.installForTesting(ByteBuddyAgent.install(),transport,AgentConfig.forTesting("sample.app"));
        exerciseNestedServlets("11111111111111111111111111111111","22222222222222222222222222222222");
        exerciseNestedServlets("33333333333333333333333333333333","33333333333333333333333333333333");
        java.util.List<Event> events=new java.util.ArrayList<Event>();for(Event event;(event=transport.pollForTesting())!=null;)events.add(event);
        java.util.Map<String,Event> byId=new java.util.HashMap<String,Event>();
        for(Event event:events)assertNull(byId.put(event.eventId,event),"event identities must be unique across trace states");
        for(Event event:events)if(event.parentEventId!=null){Event parent=byId.get(event.parentEventId);assertNotNull(parent,"parent must be present in the captured flow");assertEquals(event.traceId,parent.traceId,"causal edges cannot cross trace IDs");assertNotEquals(event.eventId,event.parentEventId,"an event cannot parent itself");}
        Event outerDifferent=http(events,"11111111111111111111111111111111",1);
        Event innerDifferent=http(events,"22222222222222222222222222222222",1);
        assertNull(outerDifferent.parentEventId);assertNull(innerDifferent.parentEventId);
        Event restoredOuter=events.stream().filter(e->e.traceId.equals(outerDifferent.traceId)&&e.kind.equals("method.start")&&"load".equals(e.metadata.get("code.method"))).findFirst().get();
        assertEquals(outerDifferent.eventId,restoredOuter.parentEventId,"after the inner trace exits, the outer context remains current");
        java.util.List<Event> outerEvents=events.stream().filter(e->e.traceId.equals(outerDifferent.traceId)).collect(java.util.stream.Collectors.toList());
        assertEquals(outerDifferent.producerId,restoredOuter.producerId);assertEquals(3,restoredOuter.sequence,"endpoint identity adds one event without advancing this trace during nested requests");
        for(int i=0;i<outerEvents.size();i++){assertEquals(i+1,outerEvents.get(i).sequence);assertEquals(outerDifferent.producerId,outerEvents.get(i).producerId);}
        Event outerSame=http(events,"33333333333333333333333333333333",1);
        Event innerSame=http(events,"33333333333333333333333333333333",3);
        assertEquals(outerSame.eventId,innerSame.parentEventId,"same-trace nested HTTP requests retain the causal edge");
        java.util.List<Event> sameTrace=events.stream().filter(e->e.traceId.equals(outerSame.traceId)).collect(java.util.stream.Collectors.toList());
        for(int i=0;i<sameTrace.size();i++)assertEquals(i+1,sameTrace.get(i).sequence);
    }
    private void exerciseNestedServlets(String outerTrace,String innerTrace)throws Exception {
        String innerParent="00-"+innerTrace+"-abcdef0123456789-01";
        Class<?> type=Class.forName("javax.servlet.fake.NestedServlet");Object servlet=type.getDeclaredConstructor(String.class).newInstance(innerParent);
        FakeServlet.Request request=new FakeServlet.TraceRequest("00-"+outerTrace+"-0123456789abcdef-01");
        type.getMethod("service",Object.class,Object.class).invoke(servlet,request,new Object());
    }
    private static Event http(java.util.List<Event> events,String trace,long sequence){return events.stream().filter(e->e.traceId.equals(trace)&&e.kind.equals("http.server")&&e.sequence==sequence).findFirst().orElseThrow(()->new AssertionError("missing HTTP event for "+trace+" sequence "+sequence+"; observed="+events.stream().map(e->e.traceId+":"+e.sequence+":"+e.kind+":"+e.parentEventId).collect(java.util.stream.Collectors.toList())));}
    private Class<?> isolatedSampleClass() throws Exception {byte[] bytes;try(InputStream in=getClass().getResourceAsStream("/sample/app/SampleApplication.class")){bytes=new byte[in.available()];int n=in.read(bytes);assertEquals(bytes.length,n);}return new ClassLoader(getClass().getClassLoader()){protected Class<?> loadClass(String name,boolean resolve)throws ClassNotFoundException{synchronized(getClassLoadingLock(name)){if(name.equals("sample.app.SampleApplication")){Class<?> c=findLoadedClass(name);if(c==null)c=defineClass(name,bytes,0,bytes.length);if(resolve)resolveClass(c);return c;}return super.loadClass(name,resolve);}}}.loadClass("sample.app.SampleApplication");}
    private Class<?> isolatedSampleClassWithoutParent() throws Exception {byte[] bytes=classBytes("/sample/app/IsolatedApplication.class");ClassLoader loader=new ClassLoader(null){protected Class<?> findClass(String name)throws ClassNotFoundException{if(!name.equals("sample.app.IsolatedApplication"))throw new ClassNotFoundException(name);return defineClass(name,bytes,0,bytes.length);}};return loader.loadClass("sample.app.IsolatedApplication");}
    private Class<?> isolatedClassWithoutDebugLine() throws Exception {byte[] bytes=new ByteBuddy().subclass(Object.class).name("sample.app.NoDebugApplication").defineMethod("run",void.class,java.lang.reflect.Modifier.PUBLIC).intercept(StubMethod.INSTANCE).make().getBytes();final boolean[] foundLine={false};new net.bytebuddy.jar.asm.ClassReader(bytes).accept(new net.bytebuddy.jar.asm.ClassVisitor(net.bytebuddy.jar.asm.Opcodes.ASM9){@Override public net.bytebuddy.jar.asm.MethodVisitor visitMethod(int access,String name,String descriptor,String signature,String[] exceptions){return new net.bytebuddy.jar.asm.MethodVisitor(net.bytebuddy.jar.asm.Opcodes.ASM9){@Override public void visitLineNumber(int line,net.bytebuddy.jar.asm.Label start){foundLine[0]=true;}};}},0);assertFalse(foundLine[0],"fixture must truly have no LineNumberTable");ClassLoader loader=new ClassLoader(null){protected Class<?> findClass(String name)throws ClassNotFoundException{if(!name.equals("sample.app.NoDebugApplication"))throw new ClassNotFoundException(name);return defineClass(name,bytes,0,bytes.length);}};return loader.loadClass("sample.app.NoDebugApplication");}
    private byte[] classBytes(String resource)throws Exception {try(InputStream in=getClass().getResourceAsStream(resource)){byte[] bytes=new byte[in.available()];int n=in.read(bytes);assertEquals(bytes.length,n);return bytes;}}
}
