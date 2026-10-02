package sample.app;

public final class EndpointServlet implements javax.servlet.Servlet {
    public void service(Object request, Object response) {
        new FacesAction().processAction(request);
    }

    public static final class Request {
        private final String traceparent;
        public Request(String traceparent) { this.traceparent = traceparent; }
        public String getHeader(String name) { return traceparent; }
        public String getMethod() { return "GET"; }
    }
}
