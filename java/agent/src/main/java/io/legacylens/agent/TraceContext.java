package io.legacylens.agent;

import java.util.ArrayDeque;
import java.util.Deque;
import java.util.concurrent.ThreadLocalRandom;

public final class TraceContext {
    private static final ThreadLocal<Deque<TraceContext>> CURRENT=new ThreadLocal<Deque<TraceContext>>();
    public final String traceId, spanId, parentSpanId;
    private TraceContext(String t,String s,String p){traceId=t;spanId=s;parentSpanId=p;}
    public static TraceContext current(){Deque<TraceContext>d=CURRENT.get();return d==null?null:d.peek();}
    static TraceContext begin(String trace,String parent){Deque<TraceContext>d=CURRENT.get();if(d==null){d=new ArrayDeque<TraceContext>();CURRENT.set(d);}TraceContext c=new TraceContext(trace,hex(8),parent);d.push(c);return c;}
    static void end(){Deque<TraceContext>d=CURRENT.get();if(d==null)return;if(!d.isEmpty())d.pop();if(d.isEmpty())CURRENT.remove();}
    static String hex(int bytes){byte[] b=new byte[bytes];ThreadLocalRandom.current().nextBytes(b);StringBuilder s=new StringBuilder();for(byte v:b)s.append(String.format("%02x",v&255));return s.toString();}
    static boolean validTraceparent(String v){return v!=null&&v.matches("00-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}")&&!v.substring(3,35).matches("0{32}")&&!v.substring(36,52).matches("0{16}");}
}
