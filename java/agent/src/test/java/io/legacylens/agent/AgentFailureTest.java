package io.legacylens.agent;

import org.junit.jupiter.api.Test;
import java.util.concurrent.*;
import com.sun.net.httpserver.HttpServer;
import java.net.InetSocketAddress;
import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.util.Collections;
import java.util.List;
import java.util.ArrayList;
import java.util.concurrent.atomic.AtomicBoolean;
import static org.junit.jupiter.api.Assertions.*;

class AgentFailureTest {
    @Test void offlineAndFullQueueDoNotBlockAndRecoveryReportsLoss() throws Exception {
        AgentTransport transport = AgentTransport.forTesting(4096);
        ExecutorService executor = Executors.newSingleThreadExecutor();
        try {
            Future<?> request = executor.submit(() -> { for (int i=0;i<4097;i++) transport.offer(Event.test("sample", i+1)); });
            request.get(2, TimeUnit.SECONDS);
            assertEquals(4096, transport.queuedForTesting());
            assertTrue(transport.droppedForTesting() > 0);
            assertNotNull(transport.pollForTesting());
            assertTrue(transport.offer(Event.test("sample", 4098)));
        } finally { executor.shutdownNow(); }
    }
    @Test void recoveryPostsLossDiagnosticToSameTraceAndProducer() throws Exception {
        AtomicBoolean online=new AtomicBoolean(false);
        List<String> bodies=Collections.synchronizedList(new ArrayList<String>());
        HttpServer server=HttpServer.create(new InetSocketAddress("127.0.0.1",0),0);
        server.createContext("/v1/events",exchange->{int code=online.get()?200:503;ByteArrayOutputStream body=new ByteArrayOutputStream();byte[] chunk=new byte[1024];for(int n;(n=exchange.getRequestBody().read(chunk))>=0;)body.write(chunk,0,n);bodies.add(new String(body.toByteArray(),"UTF-8"));exchange.sendResponseHeaders(code,-1);exchange.close();});
        server.start();
        AgentTransport transport=AgentTransport.start(AgentConfig.forTesting("sample.app","http://127.0.0.1:"+server.getAddress().getPort()+"/v1/events"));
        String traceA="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",traceB="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb";
        try {
            assertTrue(transport.offer(new Event("p1",traceA,"producer-test",transport.nextSequence(traceA),"test",null,java.util.Collections.<String,String>emptyMap())));
            awaitCount(bodies,1);
            online.set(true);
            assertTrue(transport.offer(new Event("p1",traceB,"producer-test",transport.nextSequence(traceB),"test",null,java.util.Collections.<String,String>emptyMap())));
            awaitCount(bodies,2);
            awaitCount(bodies,3);
            assertTrue(bodies.stream().anyMatch(s->s.contains("\"kind\":\"agent.loss\"")&&s.contains("\"traceId\":\""+traceA+"\"")&&s.contains("\"producerId\":\"producer-test\"")&&s.contains("\"agent.dropped_count\":\"1\"")),bodies.toString());
            assertFalse(bodies.stream().anyMatch(s->s.contains("\"traceId\":\""+traceB+"\"")&&s.contains("agent.loss")),bodies.toString());
            assertTrue(transport.recoveryDiagnosticForTesting());
        } finally {transport.closeForTesting();server.stop(0);}
    }
    private static void awaitCount(List<?> values,int count)throws InterruptedException {long deadline=System.currentTimeMillis()+4000;while(values.size()<count&&System.currentTimeMillis()<deadline)Thread.sleep(10);assertTrue(values.size()>=count,"timed out waiting for local transport requests");}
    @Test void eventSequenceIsMonotonicWithinEachTraceUnderConcurrentCalls() throws Exception {
        AgentTransport transport=AgentTransport.forTesting(8);
        assertEquals(1,transport.nextSequence("trace-a"));
        assertEquals(1,transport.nextSequence("trace-b"));
        assertEquals(2,transport.nextSequence("trace-a"));
        java.util.Set<Long> concurrent=new java.util.concurrent.ConcurrentSkipListSet<Long>();
        ExecutorService workers=Executors.newFixedThreadPool(4);
        try {java.util.List<Future<?>> tasks=new java.util.ArrayList<Future<?>>();for(int worker=0;worker<4;worker++)tasks.add(workers.submit(()->{for(int i=0;i<250;i++)concurrent.add(transport.nextSequence("trace-c"));}));for(Future<?> task:tasks)task.get(2,TimeUnit.SECONDS);}
        finally {workers.shutdownNow();}
        assertEquals(3,transport.nextSequence("trace-a"));
        assertEquals(1,concurrent.iterator().next());
        assertEquals(1000,concurrent.size());
        assertEquals(1000,concurrent.stream().mapToLong(Long::longValue).max().getAsLong());
    }
    @Test void sanitizerDropsRawSqlParametersAndExceptionText() {
        assertEquals("SELECT * FROM orders WHERE id = ?", Sanitizer.sql("SELECT * FROM orders WHERE id = ?"));
        assertEquals("", Sanitizer.sql("SELECT * FROM orders WHERE id = 42"));
        assertFalse(Sanitizer.sql("SELECT * FROM orders WHERE id = 's3cr3t'").contains("s3cr3t"));
    }
}
