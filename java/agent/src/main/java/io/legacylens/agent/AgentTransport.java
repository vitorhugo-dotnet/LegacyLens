package io.legacylens.agent;

import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.concurrent.ArrayBlockingQueue;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicLong;

public final class AgentTransport {
    private final ArrayBlockingQueue<Event> queue;
    private final ConcurrentHashMap<String, AtomicLong> sequences = new ConcurrentHashMap<String, AtomicLong>();
    private final ConcurrentHashMap<String, AtomicLong> losses = new ConcurrentHashMap<String, AtomicLong>();
    private final AgentConfig config;
    private volatile boolean closed;
    private volatile boolean recoveryDiagnostic;

    private AgentTransport(AgentConfig config, int capacity, boolean background) {
        this.config = config;
        this.queue = new ArrayBlockingQueue<Event>(capacity);
        if (background) {
            Thread sender = new Thread(new Runnable() { public void run() { drain(); } }, "legacylens-agent-transport");
            sender.setDaemon(true);
            sender.start();
        }
    }

    public static AgentTransport start(AgentConfig config) { return new AgentTransport(config, 4096, true); }
    static AgentTransport forTesting(int capacity) { return new AgentTransport(null, capacity, false); }
    public boolean offer(Event event) { if (!queue.offer(event)) { markDropped(event.traceId); return false; } return true; }
    long nextSequence(String traceId) {
        AtomicLong value = sequences.get(traceId);
        if (value == null) { AtomicLong candidate = new AtomicLong(); AtomicLong old = sequences.putIfAbsent(traceId, candidate); value = old == null ? candidate : old; }
        return value.incrementAndGet();
    }
    Event pollForTesting() { return queue.poll(); }
    int queuedForTesting() { return queue.size(); }
    long droppedForTesting() { long total=0; for(AtomicLong n:losses.values())total+=n.get(); return total; }
    boolean recoveryDiagnosticForTesting() { return recoveryDiagnostic; }
    void closeForTesting() { closed=true; }

    private void drain() {
        while (!closed) {
            Event event=null;
            try {
                event=queue.poll(250,TimeUnit.MILLISECONDS);
                if(event==null || config==null)continue;
                if(!send(event)){markDropped(event.traceId);continue;}
                reportAllLosses();
            } catch(InterruptedException ignored) { Thread.currentThread().interrupt(); return; }
              catch(Exception ignored) { if(event!=null)markDropped(event.traceId); }
        }
    }
    private void reportAllLosses() {
        for(Map.Entry<String,AtomicLong> entry:losses.entrySet()) {
            String traceId=entry.getKey();AtomicLong loss=entry.getValue();long count=loss.getAndSet(0);if(count==0)continue;
            Map<String,String> metadata=new LinkedHashMap<String,String>();metadata.put("agent.dropped_count",String.valueOf(count));
            Event diagnostic=new Event(config.projectId,traceId,config.producerId,nextSequence(traceId),"agent.loss",config.revision,metadata);
            try {
                if(send(diagnostic)){recoveryDiagnostic=true;if(loss.get()==0)losses.remove(traceId,loss);}
                else loss.addAndGet(count);
            } catch(Exception ignored) { loss.addAndGet(count);break; }
        }
    }
    private void markDropped(String traceId) {
        AtomicLong value=losses.get(traceId);
        if(value==null){AtomicLong candidate=new AtomicLong();AtomicLong old=losses.putIfAbsent(traceId,candidate);value=old==null?candidate:old;}
        value.incrementAndGet();
    }
    private boolean send(Event event)throws Exception {
        HttpURLConnection connection=(HttpURLConnection)new URL(config.endpoint).openConnection();
        connection.setConnectTimeout(250); connection.setReadTimeout(250); connection.setInstanceFollowRedirects(false);
        connection.setRequestMethod("POST"); connection.setDoOutput(true);
        connection.setRequestProperty("Authorization","Bearer "+config.token); connection.setRequestProperty("Content-Type","application/json");
        String body="{\"protocolVersion\":1,\"requestId\":"+Event.q(event.eventId)+",\"command\":\"trace.ingest\",\"payload\":{\"projectId\":"+Event.q(event.projectId)+",\"events\":["+event.toJson()+"]}}";
        try(OutputStream output=connection.getOutputStream()){output.write(body.getBytes("UTF-8"));}
        int status=connection.getResponseCode(); connection.disconnect(); return status>=200&&status<300;
    }
}
