package javax.servlet.fake;

public final class NestedServlet implements javax.servlet.Servlet {
    private final String nestedTraceparent;

    public NestedServlet(String nestedTraceparent) { this.nestedTraceparent=nestedTraceparent; }

    public void service(Object request,Object response) {
        new FakeServlet().service(new FakeServlet.TraceRequest(nestedTraceparent),new Object());
        new sample.app.SampleApplication().load(null);
    }
}
