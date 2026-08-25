/* scrollmod_host.h — real extern declaration for the one host-side symbol
 * vterm_mod.c's #target/inline-c body calls into, plus the mod's own
 * entry point. Same "-include this header before compiling the
 * generated C" pattern PARENA/examples/BUILD.bazel already established
 * for editor_plugin_host_stubs.h (see that file's own header comment) --
 * here wired in via cgo's `#cgo CFLAGS: -include` instead of Bazel's
 * `-include` copt, same mechanism, different build system.
 *
 * Unlike editor_plugin_host_stubs.h (genuinely unimplemented stubs),
 * pitviper_host_scroll has a real Go implementation just below in
 * scrollmod.go, exported via cgo `//export`.
 */
#ifndef SCROLLMOD_HOST_H
#define SCROLLMOD_HOST_H

extern void pitviper_host_scroll(int delta);
extern void on_wheel_scroll(int delta);

#endif /* SCROLLMOD_HOST_H */
