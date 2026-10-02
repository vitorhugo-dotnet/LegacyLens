package io.legacylens.agent;

import io.legacylens.agent.ApplicationIdentity;
import io.legacylens.agent.ApplicationRevision;
import org.junit.jupiter.api.Test;
import net.bytebuddy.agent.ByteBuddyAgent;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.net.URL;
import java.net.URLConnection;
import java.net.URLStreamHandler;
import java.security.CodeSource;
import java.security.ProtectionDomain;
import java.util.jar.Attributes;
import java.util.jar.Manifest;
import static org.junit.jupiter.api.Assertions.*;
import java.util.ArrayList;
import java.util.List;
import sample.app.EndpointServlet;
import sample.app.FacesAction;

class EndpointFlowTest {
    @Test void servletIdentityIsStableAndDoesNotGuessApplicationRevision() {
        ApplicationIdentity identity = ApplicationIdentity.from(EndpointFlowTest.class);
        assertEquals("io.legacylens.agent.EndpointFlowTest", identity.className());
        assertEquals("unconfirmed", ApplicationRevision.fromManifest(null).status());
        assertNull(ApplicationRevision.fromManifest(null).value());
    }

    @Test void revisionUsesOnlyManifestEvidence() {
        Manifest manifest = new Manifest();
        Attributes attributes = manifest.getMainAttributes();
        attributes.putValue("Manifest-Version", "1.0");
        attributes.putValue("Implementation-Version", "2.7.1");
        attributes.putValue("Build-Revision", "abc123");
        ApplicationRevision revision = ApplicationRevision.fromManifest(manifest);
        assertEquals("confirmed", revision.status());
        assertEquals("2.7.1+abc123", revision.value());
    }

    @Test void wildFlyVfsWarManifestConfirmsRevisionOnlyForItsOwnDeployment() throws Exception {
        Manifest manifest = new Manifest();
        manifest.getMainAttributes().putValue("Manifest-Version", "1.0");
        manifest.getMainAttributes().putValue("Implementation-Version", "3.1.4");
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        manifest.write(bytes);
        final byte[] contents = bytes.toByteArray();
        final String className = EndpointFlowTest.class.getName();
        try (InputStream source = EndpointFlowTest.class.getResourceAsStream("/" + className.replace('.', '/') + ".class")) {
            final byte[] classBytes = new byte[source.available()];
            int read = source.read(classBytes);
            assertEquals(classBytes.length, read);
            final URL codeLocation = resourceUrl("vfs:/content/orders.war/WEB-INF/classes/", new byte[0]);
            final URL manifestLocation = resourceUrl("vfs:/content/orders.war/META-INF/MANIFEST.MF", contents);
            ClassLoader loader = new ClassLoader(EndpointFlowTest.class.getClassLoader()) {
                @Override public URL getResource(String name) {
                    return "META-INF/MANIFEST.MF".equals(name) ? manifestLocation : super.getResource(name);
                }
                @Override protected Class<?> loadClass(String name, boolean resolve) throws ClassNotFoundException {
                    if (!className.equals(name)) return super.loadClass(name, resolve);
                    synchronized (getClassLoadingLock(name)) {
                        Class<?> loaded = findLoadedClass(name);
                        if (loaded == null) {
                            ProtectionDomain domain = new ProtectionDomain(new CodeSource(codeLocation, (java.security.cert.Certificate[]) null), null, this, null);
                            loaded = defineClass(name, classBytes, 0, classBytes.length, domain);
                        }
                        if (resolve) resolveClass(loaded);
                        return loaded;
                    }
                }
            };
            Class<?> deployedClass = loader.loadClass(className);
            ApplicationRevision revision = ApplicationIdentity.describe(deployedClass);
            assertEquals("confirmed", revision.status());
            assertEquals("3.1.4", revision.value());
        }
    }

    @Test void facesAdviceRecognizesActionCallbacksWithoutFacesApiTypes() {
        assertTrue(io.legacylens.agent.instrumentation.FacesAdvice.isActionCallback("javax.faces.event.ActionListener", "processAction"));
        assertTrue(io.legacylens.agent.instrumentation.FacesAdvice.isActionCallback("jakarta.faces.event.ActionListener", "processAction"));
        assertFalse(io.legacylens.agent.instrumentation.FacesAdvice.isActionCallback("org.example.ActionListener", "processAction"));
        assertTrue(io.legacylens.agent.instrumentation.FacesAdvice.isActionCallback(FacesAction.class, "processAction"));
    }

    @Test void facesAdviceInvokesBridgeForRecognizedAction() throws Exception {
        AgentTransport transport = AgentTransport.forTesting(16);
        LegacyLensAgent.installForTesting(ByteBuddyAgent.install(), transport, AgentConfig.forTesting("sample.app"));
        LegacyLensAgent.beginTraceForTesting("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb");
        try {
            boolean active = io.legacylens.agent.instrumentation.FacesAdvice.enter(FacesAction.class, "processAction");
            assertTrue(active);
            io.legacylens.agent.instrumentation.FacesAdvice.exit(active, FacesAction.class, "processAction", null);
        } finally { LegacyLensAgent.endTraceForTesting(); }
        List<Event> events = new ArrayList<Event>();
        for (Event item; (item = transport.pollForTesting()) != null;) events.add(item);
        assertNotNull(event(events, "faces.action.start"));
    }

    @Test void endpointAndFacesSpansStayNestedWithApplicationAndJdbcFlow() {
        assertTrue(io.legacylens.agent.instrumentation.FacesAdvice.isActionCallback(FacesAction.class, "processAction"));
        AgentTransport transport = AgentTransport.forTesting(32);
        LegacyLensAgent.installForTesting(ByteBuddyAgent.install(), transport, AgentConfig.forTesting("sample.app"));
        EndpointServlet servlet = new EndpointServlet();
        servlet.service(new EndpointServlet.Request("00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-0123456789abcdef-01"), new Object());

        List<Event> events = new ArrayList<Event>();
        for (Event event; (event = transport.pollForTesting()) != null;) events.add(event);
        Event http = event(events, "http.server");
        Event endpoint = event(events, "endpoint.start");
        Event faces = event(events, "faces.action.start");
        Event callback = events.stream().filter(e -> e.kind.equals("method.start") && "processAction".equals(e.metadata.get("code.method"))).findFirst().orElseThrow(AssertionError::new);
        Event service = events.stream().filter(e -> e.kind.equals("method.start") && "load".equals(e.metadata.get("code.method"))).findFirst().orElseThrow(AssertionError::new);
        Event jdbc = event(events, "db.query");
        assertNull(http.parentEventId);
        assertEquals(http.eventId, endpoint.parentEventId);
        assertEquals("unconfirmed", endpoint.metadata.get("application.revision.status"));
        assertEquals("unconfirmed", faces.metadata.get("application.revision.status"));
        assertEquals(http.eventId, faces.parentEventId);
        assertEquals(faces.eventId, callback.parentEventId);
        assertEquals(callback.eventId, service.parentEventId);
        assertEquals(service.eventId, jdbc.parentEventId);
        assertTrue(events.stream().anyMatch(e -> e.kind.equals("endpoint.end")));
        assertTrue(events.stream().anyMatch(e -> e.kind.equals("faces.action.end")));
        assertEquals("unconfirmed", ApplicationIdentity.describe(EndpointServlet.class).status(), "classes directory is not WAR manifest evidence");
    }

    private static Event event(List<Event> events, String kind) {
        return events.stream().filter(e -> e.kind.equals(kind)).findFirst().orElseThrow(() -> new AssertionError("missing " + kind + " in " + events.stream().map(e -> e.kind + "<-" + e.parentEventId).collect(java.util.stream.Collectors.toList())));
    }

    private static URL resourceUrl(String address, final byte[] contents) throws Exception {
        return new URL(null, address, new URLStreamHandler() {
            @Override protected URLConnection openConnection(URL url) {
                return new URLConnection(url) {
                    @Override public void connect() { }
                    @Override public InputStream getInputStream() { return new ByteArrayInputStream(contents); }
                };
            }
        });
    }
}
