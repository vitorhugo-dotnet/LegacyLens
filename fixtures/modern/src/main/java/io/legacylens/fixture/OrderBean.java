package io.legacylens.fixture;

import jakarta.faces.context.FacesContext;
import jakarta.faces.view.ViewScoped;
import jakarta.inject.Named;
import org.primefaces.PrimeFaces;

import java.io.Serializable;
import java.io.InputStream;

@Named("orderBean")
@ViewScoped
public class OrderBean implements Serializable {
    private static final long serialVersionUID = 1L;
    private final OrderService service = new OrderService();
    private String note;
    private String message;

    public String getNote() { return note; }
    public void setNote(String note) { this.note = note; }
    public String getMessage() { return message; }
    public int getOrderCount() { return service.count(); }
    public String getJavaVersion() { return System.getProperty("java.runtime.version", "unknown"); }
    public String getFacesVersion() { return version(FacesContext.class); }
    public String getPrimeFacesVersion() { return version(PrimeFaces.class); }
    public String getMysqlVersion() { return service.databaseVersion(); }
    public String getConnectorJVersion() { return service.driverVersion(); }
    public String getBuildRevision() {
        try (InputStream input = OrderBean.class.getResourceAsStream("/fixture-revision.txt")) {
            if (input == null) return "unknown";
            byte[] value = new byte[128];
            int length = input.read(value);
            String revision = length <= 0 ? null : new String(value, 0, length, java.nio.charset.StandardCharsets.UTF_8).trim();
            return revision == null || revision.isEmpty() ? "unknown" : revision;
        } catch (Exception ignored) { return "unknown"; }
    }
    public void save() { service.save(note); message = "Order saved"; }

    private static String version(Class<?> type) {
        Package pkg = type.getPackage();
        String version = pkg == null ? null : pkg.getImplementationVersion();
        return version == null || version.trim().isEmpty() ? "unknown" : version.trim();
    }
}
