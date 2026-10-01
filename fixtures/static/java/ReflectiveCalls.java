package sample;

class ReflectiveCalls {
    record Payload(String value) {}

    String modern(Object value) {
        return switch (value) {
            case String text -> text;
            case Integer number -> number.toString();
            default -> "unknown";
        };
    }

    void dynamic(String name) throws Exception {
        Class.forName(name);
    }
}
