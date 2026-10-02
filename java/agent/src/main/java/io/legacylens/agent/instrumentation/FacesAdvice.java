package io.legacylens.agent.instrumentation;

import net.bytebuddy.asm.Advice;

/** Traces JSF action callbacks by class name, without linking to either Faces API. */
public final class FacesAdvice {
    private FacesAdvice() { }

    public static boolean isActionCallback(String owner, String method) {
        return "processAction".equals(method)
            && ("javax.faces.event.ActionListener".equals(owner)
                || "jakarta.faces.event.ActionListener".equals(owner));
    }

    public static boolean isActionCallback(Class<?> owner, String method) {
        if (owner == null || !"processAction".equals(method)) return false;
        if (isActionCallback(owner.getName(), method)) return true;
        for (Class<?> face : owner.getInterfaces()) if (isActionCallback(face, method)) return true;
        Class<?> parent = owner.getSuperclass();
        return parent != null && isActionCallback(parent, method);
    }

    @Advice.OnMethodEnter(suppress = Throwable.class)
    public static boolean enter(@Advice.Origin Class<?> owner, @Advice.Origin("#m") String method) throws Exception {
        if (!isActionCallback(owner, method)) return false;
        return Boolean.TRUE.equals(EndpointAdvice.call("facesActionEnter", new Class<?>[] { Class.class, String.class }, owner, method));
    }

    @Advice.OnMethodExit(onThrowable = Throwable.class, suppress = Throwable.class)
    public static void exit(@Advice.Enter boolean active, @Advice.Origin Class<?> owner, @Advice.Origin("#m") String method,
                            @Advice.Thrown Throwable thrown) throws Exception {
        if (!active) return;
        EndpointAdvice.call("facesActionExit", new Class<?>[] { Class.class, String.class, Throwable.class }, owner, method, thrown);
    }
}
