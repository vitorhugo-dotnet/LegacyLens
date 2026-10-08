package io.legacylens.agent;

import java.util.ArrayDeque;
import java.util.Deque;
import java.security.SecureRandom;

public final class TraceContext {
    private static final SecureRandom RANDOM = new SecureRandom();
    private static final ThreadLocal<Deque<TraceContext>> CURRENT=new ThreadLocal<Deque<TraceContext>>();
    public final String traceId, spanId, parentSpanId;
    final AgentTransport.TraceState state;
    private final Deque<String> eventStack=new ArrayDeque<String>();
    private final String inheritedParentEventId;
    private TraceContext(String t,String s,String p,String inherited,AgentTransport.TraceState traceState){traceId=t;spanId=s;parentSpanId=p;inheritedParentEventId=inherited;state=traceState;}
    public static TraceContext current(){Deque<TraceContext>d=CURRENT.get();return d==null?null:d.peek();}
    static TraceContext begin(String trace,String parent,AgentTransport.TraceState state){Deque<TraceContext>d=CURRENT.get();if(d==null){d=new ArrayDeque<TraceContext>();CURRENT.set(d);}TraceContext enclosing=d.peek();String inherited=enclosing!=null&&enclosing.traceId.equals(trace)?enclosing.currentEventId():null;TraceContext c=new TraceContext(trace,hex(8),parent,inherited,state);d.push(c);return c;}
    static void pushEvent(String eventId){TraceContext c=current();if(c!=null&&eventId!=null)c.eventStack.push(eventId);}
    static void popEvent(){TraceContext c=current();if(c!=null&&!c.eventStack.isEmpty())c.eventStack.pop();}
    static String parentEventId(){TraceContext c=current();return c==null?null:c.currentEventId();}
    private String currentEventId(){return eventStack.isEmpty()?inheritedParentEventId:eventStack.peek();}
    static TraceContext snapshot(){TraceContext current=current();return current==null?null:new TraceContext(current.traceId,hex(8),current.spanId,current.currentEventId(),current.state);}
    static void install(TraceContext context){if(context==null)return;Deque<TraceContext>d=CURRENT.get();if(d==null){d=new ArrayDeque<TraceContext>();CURRENT.set(d);}d.push(context);}
    static TraceContext end(){Deque<TraceContext>d=CURRENT.get();if(d==null)return null;TraceContext c=d.isEmpty()?null:d.pop();if(d.isEmpty())CURRENT.remove();return c;}
    static String hex(int bytes){byte[] b=new byte[bytes];do { RANDOM.nextBytes(b); } while(allZero(b));StringBuilder s=new StringBuilder();for(byte v:b)s.append(String.format("%02x",v&255));return s.toString();}
    private static boolean allZero(byte[] b){for(byte value:b)if(value!=0)return false;return true;}
    static boolean validTraceparent(String v){return v!=null&&v.matches("00-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}")&&!v.substring(3,35).matches("0{32}")&&!v.substring(36,52).matches("0{16}");}
}
