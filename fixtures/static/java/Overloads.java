package sample;

class Overloads {
    void call(int count) {}
    void call(String value) {}
    void run() {
        call(1);
        call("value");
    }
}
