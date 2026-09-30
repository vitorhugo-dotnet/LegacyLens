package io.legacylens.agent;

import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.Map;

/** Protocol 1 event sent by the isolated Java producer. */
public final class Event {
    private static final String PROCESS_NONCE=TraceContext.hex(8);
    public final String projectId, traceId, producerId, eventId, parentEventId, kind, occurredAt, applicationRevision;
    public final long sequence;
    public final Map<String,String> metadata;
    Event(String projectId, String traceId, String producerId, long sequence, String kind, String revision, Map<String,String> metadata) {
        this.projectId=projectId; this.traceId=traceId; this.producerId=producerId; this.sequence=sequence;
        this.eventId="evt-"+PROCESS_NONCE+"-"+sequence; this.parentEventId=null; this.kind=kind;
        this.occurredAt=Instant.now().toString(); this.applicationRevision=revision;
        this.metadata=new LinkedHashMap<String,String>(metadata);
    }
    static Event test(String kind,long sequence) { return new Event("p1", "0123456789abcdef0123456789abcdef", "test-producer", sequence, kind, null, new LinkedHashMap<String,String>()); }
    public String toJson() {
        StringBuilder b=new StringBuilder("{\"projectId\":").append(q(projectId)).append(",\"traceId\":").append(q(traceId)).append(",\"producerId\":").append(q(producerId)).append(",\"sequence\":").append(sequence).append(",\"eventId\":").append(q(eventId)).append(",\"kind\":").append(q(kind)).append(",\"occurredAt\":").append(q(occurredAt));
        if(applicationRevision!=null)b.append(",\"applicationRevision\":").append(q(applicationRevision));
        b.append(",\"metadata\":{"); boolean comma=false; for(Map.Entry<String,String> e:metadata.entrySet()){if(comma)b.append(',');comma=true;b.append(q(e.getKey())).append(':').append(q(e.getValue()));} return b.append("}}").toString();
    }
    static String q(String s) { StringBuilder b=new StringBuilder("\""); for(char c:s.toCharArray()){switch(c){case '"':b.append("\\\"");break;case '\\':b.append("\\\\");break;case '\n':b.append("\\n");break;case '\r':b.append("\\r");break;case '\t':b.append("\\t");break;default:if(c<32)b.append(String.format("\\u%04x",(int)c));else b.append(c);}}return b.append('"').toString(); }
}
