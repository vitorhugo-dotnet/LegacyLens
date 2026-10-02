package sample.app;

public final class FacesAction implements javax.faces.event.ActionListener {
    public void processAction(Object event) { new SampleApplication().load(null); }
}
