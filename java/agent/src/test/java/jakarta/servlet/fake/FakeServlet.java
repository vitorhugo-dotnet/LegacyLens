package jakarta.servlet.fake;
public class FakeServlet implements jakarta.servlet.Servlet {
 public void service(Object request,Object response){new sample.app.SampleApplication().load(null);}
 public static class Request {public String getHeader(String name){return "00-1123456789abcdef0123456789abcdef-1123456789abcdef-01";}public String getMethod(){return "POST";}public String getRequestURI(){return "/private/not-recorded";}}
}
