package io.legacylens.agent;

import java.util.jar.Attributes;
import java.util.jar.Manifest;
import java.io.File;
import java.io.InputStream;
import java.net.URL;
import java.util.Locale;
import java.util.zip.ZipEntry;
import java.util.zip.ZipFile;

/** Revision derived only from explicit deployment manifest attributes. */
public final class ApplicationRevision {
    private final String value;
    private final String status;

    private ApplicationRevision(String value, String status) { this.value = value; this.status = status; }

    public static ApplicationRevision fromManifest(Manifest manifest) {
        if (manifest == null) return new ApplicationRevision(null, "unconfirmed");
        Attributes attributes = manifest.getMainAttributes();
        String version = clean(attributes.getValue("Implementation-Version"));
        String revision = clean(attributes.getValue("Build-Revision"));
        if (version == null && revision == null) return new ApplicationRevision(null, "unconfirmed");
        String value = version == null ? revision : revision == null ? version : version + "+" + revision;
        return new ApplicationRevision(value, "confirmed");
    }

    /** Reads deployment metadata only when the class and manifest identify the same WAR deployment. */
    public static ApplicationRevision fromClass(Class<?> type) {
        if (type == null || type.getProtectionDomain() == null || type.getProtectionDomain().getCodeSource() == null
                || type.getProtectionDomain().getCodeSource().getLocation() == null) return fromManifest(null);
        URL location = type.getProtectionDomain().getCodeSource().getLocation();
        try {
            if ("file".equalsIgnoreCase(location.getProtocol())) {
                File deployment = new File(location.toURI());
                if (deployment.isFile() && deployment.getName().toLowerCase(Locale.ROOT).endsWith(".war")) {
                    try (ZipFile archive = new ZipFile(deployment)) {
                        ZipEntry entry = archive.getEntry("META-INF/MANIFEST.MF");
                        if (entry == null) return fromManifest(null);
                        try (InputStream input = archive.getInputStream(entry)) { return fromManifest(new Manifest(input)); }
                    }
                }
            }
        } catch (Exception ignored) { /* Fall through to the deployment classloader resource. */ }

        try {
            URL manifestUrl = type.getResource("/META-INF/MANIFEST.MF");
            if (!sameWarDeployment(location, manifestUrl)) return fromManifest(null);
            try (InputStream input = manifestUrl.openStream()) { return fromManifest(new Manifest(input)); }
        } catch (Exception ignored) { return fromManifest(null); }
    }

    private static boolean sameWarDeployment(URL location, URL manifest) {
        String deployment = warIdentity(location);
        String resource = warIdentity(manifest);
        return deployment != null && deployment.equals(resource);
    }

    private static String warIdentity(URL url) {
        if (url == null) return null;
        String external = url.toExternalForm();
        int end = external.toLowerCase(Locale.ROOT).indexOf(".war");
        return end < 0 ? null : external.substring(0, end + 4).toLowerCase(Locale.ROOT);
    }

    private static String clean(String value) { return value == null || value.trim().isEmpty() ? null : value.trim(); }
    public String value() { return value; }
    public String status() { return status; }
}
