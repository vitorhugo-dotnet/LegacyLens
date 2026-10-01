package javax.servlet.fake;
public class FakeServlet implements javax.servlet.Servlet {
 public void service(Object request,Object response){new sample.app.SampleApplication().load(((Request)request).secret);}
 public static class Request {final String secret;public Request(){this(null);}public Request(String s){secret=s;}public String getHeader(String name){return "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01";}public String getMethod(){return "GET";}public String getRequestURI(){return "/orders/42";}}
 public static final class TraceRequest extends Request {private final String traceparent;public TraceRequest(String value){traceparent=value;}public String getHeader(String name){return traceparent;}}
}
