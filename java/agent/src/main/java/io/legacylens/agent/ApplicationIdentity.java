package io.legacylens.agent;

/** Stable application class identity; deliberately excludes request supplied values. */
public final class ApplicationIdentity {
    private final String className;

    private ApplicationIdentity(String className) { this.className = className; }

    public static ApplicationIdentity from(Class<?> type) {
        if (type == null) throw new IllegalArgumentException("type is required");
        return new ApplicationIdentity(type.getName());
    }

    /** Return only a revision supported by the application's deployment manifest. */
    public static ApplicationRevision describe(Class<?> applicationClass) {
        return ApplicationRevision.fromClass(applicationClass);
    }

    public String className() { return className; }
}
