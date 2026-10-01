package io.legacylens.agent;

import org.junit.jupiter.api.Test;
import com.sun.net.httpserver.HttpServer;
import java.net.InetSocketAddress;
import java.io.ByteArrayOutputStream;
import java.io.PrintStream;
import java.util.Collections;
import java.util.List;
import java.util.ArrayList;
import java.util.Map;
import java.util.HashMap;
import java.util.concurrent.*;
import java.util.concurrent.atomic.AtomicBoolean;
import static org.junit.jupiter.api.Assertions.*;

class AgentFailureTest {
    @Test void offlineAndFullQueueDoNotBlockApplicationOffers() throws Exception {
        AgentTransport transport = AgentTransport.forTesting(4096);
        AgentTransport.TraceState state = transport.acquireTrace("0123456789abcdef0123456789abcdef", "producer-test");
        ExecutorService executor = Executors.newSingleThreadExecutor();
        try {
            Future<?> application = executor.submit(() -> { for (int i=0;i<4097;i++) transport.offer(event(transport,state,"sample")); });
            application.get(2, TimeUnit.SECONDS);
            assertEquals(4096, transport.queuedForTesting());
            assertEquals(1, transport.droppedForTesting());
            assertNotNull(transport.pollForTesting());
            assertTrue(transport.offer(event(transport,state,"sample")));
        } finally { executor.shutdownNow(); transport.releaseTrace(state); }
    }

    @Test void recoveryPostsLossDiagnosticToSameTraceAndProducer() throws Exception {
        AtomicBoolean online = new AtomicBoolean(false);
        List<String> bodies = Collections.synchronizedList(new ArrayList<String>());
        HttpServer server = server(online,bodies,null,null);
        AgentTransport transport = AgentTransport.start(AgentConfig.forTesting("sample.app", endpoint(server)));
        String traceA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", traceB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb";
        AgentTransport.TraceState stateA = transport.acquireTrace(traceA,"producer-test");
        AgentTransport.TraceState stateB = transport.acquireTrace(traceB,"producer-test");
        try {
            assertTrue(transport.offer(event(transport,stateA,"initial")));
            awaitBody(bodies,"\"kind\":\"initial\"");
            online.set(true);
            assertTrue(transport.offer(event(transport,stateB,"recovery")));
            awaitBody(bodies,"\"kind\":\"agent.loss\"");
            String loss=bodies.stream().filter(s->s.contains("\"kind\":\"agent.loss\"")).findFirst().orElse("");
            assertTrue(loss.contains("\"traceId\":\""+traceA+"\""),bodies.toString());
            assertTrue(loss.contains("\"producerId\":\""+stateA.producerId+"\""),bodies.toString());
            assertTrue(loss.contains("\"agent.dropped_count\":\"1\""),bodies.toString());
            assertFalse(bodies.stream().anyMatch(s->s.contains("\"traceId\":\""+traceB+"\"")&&s.contains("agent.loss")),bodies.toString());
        } finally { transport.closeForTesting(); transport.releaseTrace(stateA); transport.releaseTrace(stateB); server.stop(0); }
    }

    @Test void concurrentQueueLossDuringSuccessfulRecoveryRemainsReportable() throws Exception {
        AtomicBoolean online = new AtomicBoolean(false);
        List<String> bodies = Collections.synchronizedList(new ArrayList<String>());
        CountDownLatch emptySnapshotPosted = new CountDownLatch(1), allowFinalize = new CountDownLatch(1);
        HttpServer server = server(online,bodies,null,null);
        AgentTransport transport = AgentTransport.forRecoveryTesting(
            AgentConfig.forTesting("sample.app",endpoint(server)),2,4,
            ()->{emptySnapshotPosted.countDown();try{allowFinalize.await(3,TimeUnit.SECONDS);}catch(InterruptedException e){Thread.currentThread().interrupt();}});
        AgentTransport.TraceState state = transport.acquireTrace("cccccccccccccccccccccccccccccccc","producer-test");
        try {
            assertTrue(transport.offer(event(transport,state,"offline")));
            awaitBody(bodies,"\"kind\":\"offline\"");
            online.set(true);
            assertTrue(transport.offer(event(transport,state,"recovery")));
            assertTrue(emptySnapshotPosted.await(4,TimeUnit.SECONDS),"loss report did not reach its post-send accounting boundary");
            assertTrue(transport.offer(event(transport,state,"queued-1")));
            assertTrue(transport.offer(event(transport,state,"queued-2")));
            assertFalse(transport.offer(event(transport,state,"overflow-during-recovery")));
        } finally { allowFinalize.countDown(); }
        try {
            awaitMatchingCount(bodies,"\"kind\":\"agent.loss\"",2);
            awaitBody(bodies,"\"kind\":\"queued-2\"");
            long lossReports=bodies.stream().filter(s->s.contains("\"kind\":\"agent.loss\"")&&s.contains("\"traceId\":\"cccccccccccccccccccccccccccccccc\"")&&s.contains("\"agent.dropped_count\":\"1\"")).count();
            assertEquals(2,lossReports,"the overflow concurrent with successful loss delivery must remain in the state and be reported");
            transport.releaseTrace(state);
            awaitTraceRetirement(transport);
        } finally { transport.closeForTesting(); transport.releaseTrace(state); server.stop(0); }
    }

    @Test void traceSequenceStateRetiresWithNewProducerEpochAndCapacityIsGlobal() {
        AgentTransport transport=AgentTransport.forTesting(8,2);
        AgentTransport.TraceState a=transport.acquireTrace("trace-a","configured-producer");
        AgentTransport.TraceState b=transport.acquireTrace("trace-b","configured-producer");
        assertNotNull(a);assertNotNull(b);assertNull(transport.acquireTrace("trace-c","configured-producer"));
        assertEquals(1,transport.capacityRejectedTraceCount());
        assertEquals(1,transport.nextSequence(a));assertEquals(1,transport.nextSequence(b));
        AgentTransport.TraceState sameTrace=transport.acquireTrace("trace-a","configured-producer");
        assertSame(a,sameTrace);assertEquals(2,transport.nextSequence(sameTrace));
        transport.releaseTrace(b);
        assertEquals(1,transport.traceStateCountForTesting(),"inactive state retires only after context references close");
        AgentTransport.TraceState bAgain=transport.acquireTrace("trace-b","configured-producer");
        assertNotNull(bAgain);assertNotEquals(b.producerId,bAgain.producerId,"reused trace receives a distinct producer epoch");
        assertEquals(1,transport.nextSequence(bAgain));
        assertEquals(1,transport.capacityRejectedTraceCount());
        transport.releaseTrace(a);assertEquals(3,transport.nextSequence(sameTrace));
        transport.releaseTrace(sameTrace);transport.releaseTrace(bAgain);
        assertEquals(0,transport.traceStateCountForTesting());
        AgentTransport.TraceState c=transport.acquireTrace("trace-c","configured-producer");
        assertNotNull(c,"settled state release frees capacity for later captures");transport.releaseTrace(c);
    }

    @Test void capacityRejectionsAreObservableOffThreadAndCoalescedWithoutLosingRacingCounts() throws Exception {
        PrintStream original=System.err;BlockingDiagnosticStream diagnostics=new BlockingDiagnosticStream();
            AgentTransport transport=AgentTransport.start(AgentConfig.forTesting("sample.app","http://127.0.0.1:1/v1/events"));
        System.setErr(diagnostics);
        try {
            for(int i=1;i<=AgentTransport.DEFAULT_TRACE_STATE_CAPACITY;i++)
                assertNotNull(transport.acquireTrace(String.format("%032x",i),"configured-producer"));
            assertNull(transport.acquireTrace("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","configured-producer"));
            assertNull(transport.acquireTrace("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","configured-producer"));
            assertTrue(diagnostics.entered.await(3,TimeUnit.SECONDS),"sender must publish a global capacity diagnostic");
            String applicationThread=Thread.currentThread().getName();long start=System.nanoTime();
            assertNull(transport.acquireTrace("cccccccccccccccccccccccccccccccc","configured-producer"));
            assertNull(transport.acquireTrace("dddddddddddddddddddddddddddddddd","configured-producer"));
            assertTrue(TimeUnit.NANOSECONDS.toMillis(System.nanoTime()-start)<250,"a stalled diagnostic sink must not block application capture");
            diagnostics.release.countDown();
            long deadline=System.currentTimeMillis()+4000;while(diagnostics.lines.size()<2&&System.currentTimeMillis()<deadline)Thread.sleep(10);
            assertEquals(2,diagnostics.lines.size(),"rate limiter emits one coalesced diagnostic per interval");
            assertEquals("{\"kind\":\"agent.capacity_rejected\",\"count\":2}",diagnostics.lines.get(0));
            assertEquals("{\"kind\":\"agent.capacity_rejected\",\"count\":2}",diagnostics.lines.get(1),"rejections racing a blocked report remain pending for the next report");
            assertTrue(diagnostics.threads.stream().allMatch(thread->thread.equals("legacylens-agent-transport")));
            assertFalse(diagnostics.threads.contains(applicationThread));
            for(String line:diagnostics.lines){assertFalse(line.contains("aaaaaaaa"));assertFalse(line.contains("bbbbbbbb"));assertFalse(line.contains("cccccccc"));assertFalse(line.contains("dddddddd"));assertFalse(line.contains("configured-producer"));assertFalse(line.contains("0123456789abcdef"));}
        } finally { diagnostics.release.countDown();transport.closeForTesting();System.setErr(original); }
    }

    @Test void sanitizerDropsRawSqlParametersAndExceptionText() {
        assertEquals("SELECT * FROM orders WHERE id = ?", Sanitizer.sql("SELECT * FROM orders WHERE id = ?"));
        assertEquals("", Sanitizer.sql("SELECT * FROM orders WHERE id = 42"));
        assertFalse(Sanitizer.sql("SELECT * FROM orders WHERE id = 's3cr3t'").contains("s3cr3t"));
    }

    private static Event event(AgentTransport transport,AgentTransport.TraceState state,String kind) {
        long sequence=transport.nextSequence(state);
        return new Event("p1",state.traceId,state.producerId,sequence,kind,null,new HashMap<String,String>());
    }

    private static final class BlockingDiagnosticStream extends PrintStream {
        final List<String> lines=new CopyOnWriteArrayList<String>();
        final List<String> threads=new CopyOnWriteArrayList<String>();
        final CountDownLatch entered=new CountDownLatch(1),release=new CountDownLatch(1);
        BlockingDiagnosticStream(){super(new ByteArrayOutputStream(),true);}
        @Override public void println(String line){lines.add(line);threads.add(Thread.currentThread().getName());if(lines.size()==1){entered.countDown();try{release.await(3,TimeUnit.SECONDS);}catch(InterruptedException e){Thread.currentThread().interrupt();}}}
    }

    private static String endpoint(HttpServer server) { return "http://127.0.0.1:"+server.getAddress().getPort()+"/v1/events"; }

    private static HttpServer server(AtomicBoolean online,List<String> bodies,CountDownLatch entered,CountDownLatch release)throws Exception {
        HttpServer server=HttpServer.create(new InetSocketAddress("127.0.0.1",0),0);
        server.createContext("/v1/events",exchange->{ByteArrayOutputStream body=new ByteArrayOutputStream();byte[] chunk=new byte[1024];for(int n;(n=exchange.getRequestBody().read(chunk))>=0;)body.write(chunk,0,n);String text=new String(body.toByteArray(),"UTF-8");boolean accepted=online.get();if(entered!=null&&text.contains("agent.loss")){entered.countDown();try{release.await(3,TimeUnit.SECONDS);}catch(InterruptedException e){Thread.currentThread().interrupt();}}exchange.sendResponseHeaders(accepted?200:503,-1);exchange.close();bodies.add(text);});
        server.start();return server;
    }

    private static void awaitCount(List<?> values,int count) throws InterruptedException {long deadline=System.currentTimeMillis()+5000;while(values.size()<count&&System.currentTimeMillis()<deadline)Thread.sleep(10);assertTrue(values.size()>=count,"timed out waiting for local transport requests");}
    private static void awaitBody(List<String> bodies,String fragment) throws InterruptedException {awaitMatchingCount(bodies,fragment,1);}
    private static void awaitMatchingCount(List<String> bodies,String fragment,long count) throws InterruptedException {long deadline=System.currentTimeMillis()+5000;while(matchingCount(bodies,fragment)<count&&System.currentTimeMillis()<deadline)Thread.sleep(10);assertTrue(matchingCount(bodies,fragment)>=count,"timed out waiting for "+count+" requests containing "+fragment);}
    private static long matchingCount(List<String> bodies,String fragment) {synchronized(bodies){return bodies.stream().filter(s->s.contains(fragment)).count();}}
    private static void awaitTraceRetirement(AgentTransport transport) throws InterruptedException {long deadline=System.currentTimeMillis()+5000;while(transport.traceStateCountForTesting()!=0&&System.currentTimeMillis()<deadline)Thread.sleep(10);assertEquals(0,transport.traceStateCountForTesting(),"settled trace state must retire after events and loss reports complete");}
}
