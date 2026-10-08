package io.legacylens.agent;

import net.bytebuddy.agent.ByteBuddyAgent;
import org.junit.jupiter.api.Test;
import java.util.concurrent.*;
import java.util.concurrent.atomic.AtomicReference;
import static org.junit.jupiter.api.Assertions.*;

class AsyncContextTest {
    @Test void executorTasksPropagateOnlyTheSubmittingTraceAndClearAfterFailure() throws Exception {
        AgentTransport transport = AgentTransport.forTesting(32);
        LegacyLensAgent.installForTesting(ByteBuddyAgent.install(), transport, AgentConfig.forTesting("sample.app"));
        ExecutorService executor = Executors.newSingleThreadExecutor();
        try {
            CountDownLatch started = new CountDownLatch(1), release = new CountDownLatch(1);
            AtomicReference<TraceContext> beforeRequest = new AtomicReference<TraceContext>();
            executor.execute(() -> { beforeRequest.set(TraceContext.current()); started.countDown(); try { release.await(); } catch (InterruptedException e) { Thread.currentThread().interrupt(); } });
            assertTrue(started.await(2, TimeUnit.SECONDS));
            LegacyLensAgent.beginTraceForTesting("0123456789abcdef0123456789abcdef");
            CountDownLatch propagated = new CountDownLatch(1);
            AtomicReference<String> traceId = new AtomicReference<String>();
            executor.execute(() -> { TraceContext current = TraceContext.current(); traceId.set(current == null ? null : current.traceId); propagated.countDown(); });
            LegacyLensAgent.endTraceForTesting();
            release.countDown();
            assertTrue(propagated.await(2, TimeUnit.SECONDS));
            assertEquals("0123456789abcdef0123456789abcdef", traceId.get());
            AtomicReference<TraceContext> subsequent = new AtomicReference<TraceContext>();
            executor.submit(() -> subsequent.set(TraceContext.current())).get(2, TimeUnit.SECONDS);
            assertNull(subsequent.get());
            assertNull(beforeRequest.get());
        } finally { executor.shutdownNow(); }
    }

    @Test void exceptionInPropagatedTaskClearsWorkerContext() throws Exception {
        AgentTransport transport = AgentTransport.forTesting(32);
        LegacyLensAgent.installForTesting(ByteBuddyAgent.install(), transport, AgentConfig.forTesting("sample.app"));
        LegacyLensAgent.beginTraceForTesting("abcdef0123456789abcdef0123456789");
        Runnable task = LegacyLensAgent.wrapRunnableForAsync(() -> { throw new IllegalStateException("expected"); });
        LegacyLensAgent.endTraceForTesting();
        assertThrows(IllegalStateException.class, task::run);
        assertNull(TraceContext.current(), "the captured context must be removed even when the task throws");
    }
}
