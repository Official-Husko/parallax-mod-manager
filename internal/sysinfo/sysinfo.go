// Package sysinfo reads a few facts about the computer the app runs on - the operating
// system, the CPU, the memory, the graphics card, the session and the webview - so the
// start of the activity log can carry them (see App.logSystemReport). A bug report that
// says "it is slow" or "the window is blank" means something different on 4 cores and
// 32, or on Wayland and X11, and the person reading it shouldn't have to ask.
//
// It is meant to be cheap and safe:
//
//   - Nothing here spawns a process or touches the network: every source is a file the
//     operating system already keeps (or, on Windows, the registry), read once.
//   - Every field is optional. A source that is missing, unreadable or in an unexpected
//     shape leaves its field empty and never fails the rest.
//   - Nothing identifying is collected: no computer name, user name, serial numbers,
//     addresses or machine id.
//
// The information only goes into the local log. Sharing a log is a separate, opt-in
// step (docs/log-sharing.md).
package sysinfo

import (
	"fmt"
	"runtime"
	"strings"
)

// GPU is one graphics adapter.
type GPU struct {
	// Name is the vendor and model ("NVIDIA GeForce RTX 3070"), or the vendor and a
	// device id when the model could not be looked up.
	Name string
	// Driver is the kernel or vendor driver, with its version when that is known
	// ("nvidia 595.58.03", "amdgpu").
	Driver string
}

// Info is what was read. A zero field means it could not be found out.
type Info struct {
	// OS is the operating system's own name for itself ("EndeavourOS", "Windows 11 Pro
	// 23H2", "macOS 14.5"); Kernel is its kernel or build ("6.18.49-2-lts").
	OS     string
	Kernel string
	// Session is "Wayland" or "X11" (Linux only); Desktop the desktop environment.
	Session string
	Desktop string
	// Sandbox names a packaging that confines the app ("Flatpak", "Snap", "AppImage").
	Sandbox string
	// Webview is the engine the window is drawn with ("WebKitGTK 2.50.0",
	// "WebView2 126.0.2592.87").
	Webview string
	// CPU is the processor's model name and Threads how many the app can use.
	CPU     string
	Threads int
	// MemoryTotal and MemoryAvailable are in bytes.
	MemoryTotal     int64
	MemoryAvailable int64
	GPUs            []GPU
	// Limits are operating-system limits that matter to the app, such as how many
	// folders it may watch for changes.
	Limits []string
}

// Collect reads what this platform offers. It never returns an error: what could not
// be read is simply left empty.
func Collect() Info {
	info := collect()
	if info.Threads == 0 {
		info.Threads = runtime.NumCPU()
	}
	return info
}

// Lines renders the information as short "Label: value" lines for the log, in a fixed
// order, leaving out what is unknown.
func (i Info) Lines() []string {
	var out []string
	add := func(label, value string) {
		if strings.TrimSpace(value) != "" {
			out = append(out, label+": "+value)
		}
	}

	os := i.OS
	if os != "" && i.Kernel != "" {
		os += " (" + i.Kernel + ")"
	} else if os == "" {
		os = i.Kernel
	}
	add("OS", os)

	session := strings.TrimSpace(strings.Join(nonEmpty(i.Session, i.Desktop), ", "))
	add("Session", session)
	add("Package", i.Sandbox)
	add("Webview", i.Webview)

	cpu := i.CPU
	if i.Threads > 0 {
		if cpu != "" {
			cpu += ", "
		}
		cpu += fmt.Sprintf("%d threads", i.Threads)
	}
	add("CPU", cpu)

	if i.MemoryTotal > 0 {
		mem := HumanBytes(i.MemoryTotal) + " total"
		if i.MemoryAvailable > 0 {
			mem += ", " + HumanBytes(i.MemoryAvailable) + " available"
		}
		add("Memory", mem)
	}
	for _, g := range i.GPUs {
		v := g.Name
		if g.Driver != "" {
			v += ", driver " + g.Driver
		}
		add("GPU", v)
	}
	if len(i.Limits) > 0 {
		add("Limits", strings.Join(i.Limits, ", "))
	}
	return out
}

func nonEmpty(ss ...string) []string {
	var out []string
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// HumanBytes formats a size with binary units ("31.2 GiB").
func HumanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
