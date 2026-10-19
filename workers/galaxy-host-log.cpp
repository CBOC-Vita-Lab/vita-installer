#include <cstdint>
#include <cstdarg>
#include <cstdio>
extern "C" void port_log(const char* format, ...) {
    va_list args;
    va_start(args, format);
    vfprintf(stderr, format, args);
    va_end(args);
    fputc('\n', stderr);
}
extern "C" int64_t port_thread_cpu_us() { return 0; }
