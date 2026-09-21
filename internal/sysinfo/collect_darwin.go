package sysinfo

import "golang.org/x/sys/unix"

// The macOS sources are sysctl values. They have not been run on a Mac yet (only
// compiled). The graphics card is left out: the only ways to read it are slow.

func collect() Info {
	var info Info
	if v, err := unix.Sysctl("kern.osproductversion"); err == nil {
		info.OS = "macOS " + v
	}
	if v, err := unix.Sysctl("kern.osrelease"); err == nil {
		info.Kernel = "Darwin " + v
	}
	if v, err := unix.Sysctl("machdep.cpu.brand_string"); err == nil {
		info.CPU = collapseSpaces(v)
	} else if v, err := unix.Sysctl("hw.model"); err == nil {
		info.CPU = v
	}
	if n, err := unix.SysctlUint64("hw.memsize"); err == nil {
		info.MemoryTotal = int64(n)
	}
	info.Webview = "WKWebView (the system's WebKit)"
	return info
}
