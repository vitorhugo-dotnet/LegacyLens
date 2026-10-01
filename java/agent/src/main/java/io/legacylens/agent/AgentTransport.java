package io.legacylens.agent;

import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ArrayBlockingQueue;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicLong;

public final class AgentTransport {
    static final int DEFAULT_QUEUE_CAPACITY = 4096;
    static final int DEFAULT_TRACE_STATE_CAPACITY = 4096;
    private static final String PROCESS_NONCE = TraceContext.hex(8);
    private static final AtomicLong PRODUCER_EPOCH = new AtomicLong();
    private static final long CAPACITY_DIAGNOSTIC_INTERVAL_NANOS = TimeUnit.SECONDS.toNanos(1L);

    private static final class PendingEvent {
        final Event event;
        final TraceState state;
        PendingEvent(Event event, TraceState state) { this.event = event; this.state = state; }
    }

    static final class TraceState {
        final String traceId;
        final String producerId;
        long sequence;
        int activeContexts;
        int pendingEvents;
        long droppedEvents;
        boolean reportingLoss;
        boolean retired;
        TraceState(String traceId, String producerId) { this.traceId = traceId; this.producerId = producerId; }
    }

    private final ArrayBlockingQueue<PendingEvent> queue;
    private final Map<String, TraceState> traceStates = new HashMap<String, TraceState>();
    private final Object stateLock = new Object();
    private final int traceStateCapacity;
    private final AgentConfig config;
    private final Runnable afterLossPostedForTesting;
    private final AtomicLong capacityRejected = new AtomicLong();
    private final AtomicLong pendingCapacityRejected = new AtomicLong();
    private long nextCapacityDiagnosticAtNanos;
    private volatile boolean closed;
    private volatile boolean recoveryDiagnostic;

    private AgentTransport(AgentConfig config, int queueCapacity, int traceStateCapacity, boolean background, Runnable afterLossPostedForTesting) {
        this.config = config;
        this.queue = new ArrayBlockingQueue<PendingEvent>(queueCapacity);
        this.traceStateCapacity = traceStateCapacity;
        this.afterLossPostedForTesting = afterLossPostedForTesting;
        if (background) {
            Thread sender = new Thread(new Runnable() { public void run() { drain(); } }, "legacylens-agent-transport");
            sender.setDaemon(true);
            sender.start();
        }
    }

    public static AgentTransport start(AgentConfig config) {
        return new AgentTransport(config, DEFAULT_QUEUE_CAPACITY, DEFAULT_TRACE_STATE_CAPACITY, true, null);
    }

    static AgentTransport forTesting(int queueCapacity) {
        return new AgentTransport(null, queueCapacity, DEFAULT_TRACE_STATE_CAPACITY, false, null);
    }

    static AgentTransport forTesting(int queueCapacity, int traceStateCapacity) {
        return new AgentTransport(null, queueCapacity, traceStateCapacity, false, null);
    }

    static AgentTransport forRecoveryTesting(AgentConfig config, int queueCapacity, int traceStateCapacity, Runnable afterLossPosted) {
        return new AgentTransport(config, queueCapacity, traceStateCapacity, true, afterLossPosted);
    }

    TraceState acquireTrace(String traceId, String configuredProducerId) {
        synchronized (stateLock) {
            TraceState state = traceStates.get(traceId);
            if (state != null) {
                state.activeContexts++;
                return state;
            }
            if (traceStates.size() >= traceStateCapacity) {
                recordCapacityRejection();
                return null;
            }
            String producerId = configuredProducerId + "-" + PROCESS_NONCE + "-" + PRODUCER_EPOCH.incrementAndGet();
            state = new TraceState(traceId, producerId);
            state.activeContexts = 1;
            traceStates.put(traceId, state);
            return state;
        }
    }

    private TraceState stateForEvent(Event event) {
        synchronized (stateLock) {
            TraceState state = traceStates.get(event.traceId);
            if (state != null) return state;
            if (traceStates.size() >= traceStateCapacity) {
                recordCapacityRejection();
                return null;
            }
            state = new TraceState(event.traceId, event.producerId);
            traceStates.put(event.traceId, state);
            return state;
        }
    }

    public boolean offer(Event event) {
        TraceState state = stateForEvent(event);
        if (state == null) return false;
        synchronized (stateLock) {
            if (state.retired) return false;
            state.sequence = Math.max(state.sequence, event.sequence);
            state.pendingEvents++;
        }
        return enqueueReserved(event, state);
    }

    Event emit(TraceState state, String projectId, String revision, String kind, Map<String, String> metadata, String parentEventId) {
        long sequence;
        synchronized (stateLock) {
            if (state == null || state.retired || traceStates.get(state.traceId) != state) return null;
            sequence = ++state.sequence;
            state.pendingEvents++;
        }
        Event event = new Event(projectId, state.traceId, state.producerId, sequence, kind, revision, metadata, parentEventId);
        enqueueReserved(event, state);
        return event;
    }

    Event emitForTesting(TraceState state, String kind) {
        String projectId = config == null ? "p1" : config.projectId;
        String revision = config == null ? null : config.revision;
        return emit(state, projectId, revision, kind, new LinkedHashMap<String, String>(), null);
    }

    private boolean enqueueReserved(Event event, TraceState state) {
        if (!queue.offer(new PendingEvent(event, state))) {
            synchronized (stateLock) {
                state.droppedEvents++;
                state.pendingEvents--;
                retireIfSettled(state);
            }
            return false;
        }
        return true;
    }

    private void recordCapacityRejection() {
        capacityRejected.incrementAndGet();
        pendingCapacityRejected.incrementAndGet();
    }

    long nextSequence(TraceState state) {
        synchronized (stateLock) { return ++state.sequence; }
    }

    void releaseTrace(TraceState state) {
        synchronized (stateLock) {
            if (state.activeContexts > 0) state.activeContexts--;
            retireIfSettled(state);
        }
    }

    private void retireIfSettled(TraceState state) {
        if (state.activeContexts == 0 && state.pendingEvents == 0 && state.droppedEvents == 0 && !state.reportingLoss) {
            if (traceStates.remove(state.traceId, state)) state.retired = true;
        }
    }

    Event pollForTesting() {
        PendingEvent pending = queue.poll();
        if (pending == null) return null;
        synchronized (stateLock) {
            pending.state.pendingEvents--;
            retireIfSettled(pending.state);
        }
        return pending.event;
    }

    int queuedForTesting() { return queue.size(); }
    int traceStateCountForTesting() { synchronized (stateLock) { return traceStates.size(); } }
    long droppedForTesting() { synchronized (stateLock) { long total = 0; for (TraceState state : traceStates.values()) total += state.droppedEvents; return total; } }
    long capacityRejectedTraceCount() { return capacityRejected.get(); }
    boolean recoveryDiagnosticForTesting() { return recoveryDiagnostic; }
    void closeForTesting() { closed = true; }

    private void drain() {
        while (!closed) {
            PendingEvent pending = null;
            try {
                pending = queue.poll(100, TimeUnit.MILLISECONDS);
                reportCapacityRejections();
                if (pending == null) {
                    reportPendingLosses();
                    continue;
                }
                if (send(pending.event)) {
                    completeEvent(pending.state);
                    reportLoss(pending.state);
                } else {
                    failEvent(pending.state);
                }
            } catch (InterruptedException ignored) {
                Thread.currentThread().interrupt(); return;
            } catch (Exception ignored) {
                if (pending != null) failEvent(pending.state);
            }
        }
    }

    private void reportCapacityRejections() {
        long now=System.nanoTime();
        if(now<nextCapacityDiagnosticAtNanos)return;
        long count=pendingCapacityRejected.getAndSet(0L);
        if(count==0L)return;
        nextCapacityDiagnosticAtNanos=now+CAPACITY_DIAGNOSTIC_INTERVAL_NANOS;
        try { System.err.println("{\"kind\":\"agent.capacity_rejected\",\"count\":"+count+"}"); }
        catch(RuntimeException ignored) { pendingCapacityRejected.addAndGet(count); }
    }

    private void completeEvent(TraceState state) {
        synchronized (stateLock) {
            state.pendingEvents--;
            retireIfSettled(state);
        }
    }

    private void failEvent(TraceState state) {
        synchronized (stateLock) {
            state.droppedEvents++;
            state.pendingEvents--;
        }
    }

    private void reportPendingLosses() {
        List<TraceState> states;
        synchronized (stateLock) { states = new ArrayList<TraceState>(traceStates.values()); }
        for (TraceState state : states) reportLoss(state);
    }

    private void reportLoss(TraceState state) {
        long count;
        Event diagnostic;
        synchronized (stateLock) {
            if (state.retired || state.reportingLoss || state.droppedEvents == 0) return;
            count = state.droppedEvents;
            state.droppedEvents = 0;
            state.reportingLoss = true;
            long sequence = ++state.sequence;
            state.pendingEvents++;
            Map<String, String> metadata = new LinkedHashMap<String, String>();
            metadata.put("agent.dropped_count", String.valueOf(count));
            diagnostic = new Event(config.projectId, state.traceId, state.producerId, sequence, "agent.loss", config.revision, metadata);
        }
        boolean sent = false;
        try {
            sent = send(diagnostic);
            if (sent && afterLossPostedForTesting != null) afterLossPostedForTesting.run();
        } catch (Exception ignored) { }
        synchronized (stateLock) {
            if (sent) recoveryDiagnostic = true;
            else state.droppedEvents += count;
            state.reportingLoss = false;
            state.pendingEvents--;
            retireIfSettled(state);
        }
    }

    private boolean send(Event event) throws Exception {
        HttpURLConnection connection = (HttpURLConnection) new URL(config.endpoint).openConnection();
        connection.setConnectTimeout(250); connection.setReadTimeout(250); connection.setInstanceFollowRedirects(false);
        connection.setRequestMethod("POST"); connection.setDoOutput(true);
        connection.setRequestProperty("Authorization", "Bearer " + config.token); connection.setRequestProperty("Content-Type", "application/json");
        String body = "{\"protocolVersion\":1,\"requestId\":" + Event.q(event.eventId) + ",\"command\":\"trace.ingest\",\"payload\":{\"projectId\":" + Event.q(event.projectId) + ",\"events\":[" + event.toJson() + "]}}";
        try (OutputStream output = connection.getOutputStream()) { output.write(body.getBytes("UTF-8")); }
        int status = connection.getResponseCode(); connection.disconnect(); return status >= 200 && status < 300;
    }
}
