package sysinfo

import (
	"bufio"
	"bytes"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// The Linux sources are plain files, read through fs.FS rooted at "/" so tests can supply
// a made-up machine. Paths are therefore relative ("etc/os-release"). This file has no
// build tag so it can be tested anywhere; only collect_linux.go uses it for real.

// collectLinux reads a Linux machine described by fsys (its file tree) and env (its
// environment variables).
func collectLinux(fsys fs.FS, env func(string) string) Info {
	var info Info

	osr := parseKeyValues(readFile(fsys, "etc/os-release"))
	switch {
	case osr["NAME"] != "":
		// PRETTY_NAME often adds a rolling-release tag ("Arch Linux"); NAME is the
		// distribution, and VERSION_ID (when there is one) its release.
		info.OS = osr["NAME"]
		if v := osr["VERSION_ID"]; v != "" {
			info.OS += " " + v
		}
	case osr["PRETTY_NAME"] != "":
		info.OS = osr["PRETTY_NAME"]
	}
	info.Kernel = strings.TrimSpace(readFile(fsys, "proc/sys/kernel/osrelease"))
	if info.Kernel != "" {
		info.Kernel = "Linux " + info.Kernel
	}

	info.Session, info.Desktop = linuxSession(env)
	info.Sandbox = linuxSandbox(env)
	info.Webview = linuxWebview(fsys)

	info.CPU, info.Threads = parseCPUInfo(readFile(fsys, "proc/cpuinfo"))
	info.MemoryTotal, info.MemoryAvailable = parseMemInfo(readFile(fsys, "proc/meminfo"))
	info.GPUs = linuxGPUs(fsys)

	if w := strings.TrimSpace(readFile(fsys, "proc/sys/fs/inotify/max_user_watches")); w != "" {
		info.Limits = append(info.Limits, "inotify watches "+w)
	}
	if n := strings.TrimSpace(readFile(fsys, "proc/sys/fs/inotify/max_user_instances")); n != "" {
		info.Limits = append(info.Limits, "inotify instances "+n)
	}
	return info
}

func readFile(fsys fs.FS, name string) string {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return ""
	}
	return string(data)
}

// parseKeyValues reads "KEY=value" lines (os-release, uevent), unquoting values.
func parseKeyValues(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	return out
}

func linuxSession(env func(string) string) (session, desktop string) {
	switch strings.ToLower(env("XDG_SESSION_TYPE")) {
	case "wayland":
		session = "Wayland"
	case "x11":
		session = "X11"
	case "tty", "":
		switch {
		case env("WAYLAND_DISPLAY") != "":
			session = "Wayland"
		case env("DISPLAY") != "":
			session = "X11"
		}
	}
	desktop = env("XDG_CURRENT_DESKTOP")
	return session, desktop
}

func linuxSandbox(env func(string) string) string {
	switch {
	case env("FLATPAK_ID") != "":
		return "Flatpak"
	case env("SNAP") != "":
		return "Snap"
	case env("APPIMAGE") != "":
		return "AppImage"
	}
	return ""
}

var pcVersion = regexp.MustCompile(`(?m)^Version:\s*(\S+)`)

// linuxWebview reads the WebKitGTK version from its pkg-config file, the one place a
// running system states it without running anything.
func linuxWebview(fsys fs.FS) string {
	dirs := []string{"usr/lib/pkgconfig", "usr/lib64/pkgconfig", "usr/lib/x86_64-linux-gnu/pkgconfig", "usr/lib/aarch64-linux-gnu/pkgconfig", "usr/share/pkgconfig"}
	for _, name := range []string{"webkit2gtk-4.1.pc", "webkit2gtk-4.0.pc", "webkitgtk-6.0.pc"} {
		for _, dir := range dirs {
			if m := pcVersion.FindStringSubmatch(readFile(fsys, dir+"/"+name)); m != nil {
				return "WebKitGTK " + m[1]
			}
		}
	}
	return ""
}

// parseCPUInfo returns the processor's model name and how many logical processors are
// listed in /proc/cpuinfo.
func parseCPUInfo(text string) (model string, threads int) {
	for _, line := range strings.Split(text, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "processor":
			threads++
		case "model name", "Model", "Hardware", "cpu model":
			if model == "" {
				model = collapseSpaces(v)
			}
		}
	}
	return model, threads
}

func collapseSpaces(s string) string { return strings.Join(strings.Fields(s), " ") }

// parseMemInfo returns MemTotal and MemAvailable from /proc/meminfo, in bytes.
func parseMemInfo(text string) (total, available int64) {
	for _, line := range strings.Split(text, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields := strings.Fields(v)
		if len(fields) == 0 {
			continue
		}
		n, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			continue
		}
		if len(fields) > 1 && strings.EqualFold(fields[1], "kB") {
			n *= 1024
		}
		switch k {
		case "MemTotal":
			total = n
		case "MemAvailable":
			available = n
		}
	}
	return total, available
}

var cardDir = regexp.MustCompile(`^card[0-9]+$`)

// linuxGPUs lists the display adapters the kernel knows (sys/class/drm/cardN), naming
// each from the PCI id lists the system ships and adding its driver.
func linuxGPUs(fsys fs.FS) []GPU {
	entries, err := fs.ReadDir(fsys, "sys/class/drm")
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if cardDir.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var out []GPU
	seen := map[string]bool{}
	for _, card := range names {
		dev := "sys/class/drm/" + card + "/device/"
		vendor := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(readFile(fsys, dev+"vendor")), "0x"))
		device := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(readFile(fsys, dev+"device")), "0x"))
		if vendor == "" {
			continue
		}
		key := vendor + ":" + device
		if seen[key] {
			continue
		}
		seen[key] = true

		driver := parseKeyValues(readFile(fsys, dev+"uevent"))["DRIVER"]
		if driver == "nvidia" {
			if v := nvidiaVersion(readFile(fsys, "proc/driver/nvidia/version")); v != "" {
				driver += " " + v
			}
		}
		out = append(out, GPU{Name: gpuName(fsys, vendor, device), Driver: driver})
	}
	return out
}

var nvidiaVer = regexp.MustCompile(`\b(\d{2,3}\.\d+(?:\.\d+)?)\b`)

// nvidiaVersion pulls the driver version out of /proc/driver/nvidia/version's first
// line ("NVRM version: NVIDIA UNIX x86_64 Kernel Module  595.58.03  ...").
func nvidiaVersion(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	if m := nvidiaVer.FindStringSubmatch(line); m != nil {
		return m[1]
	}
	return ""
}

var knownVendors = map[string]string{"10de": "NVIDIA", "1002": "AMD", "1022": "AMD", "8086": "Intel", "1af4": "Virtio", "15ad": "VMware"}

// gpuName is "Vendor Model" from the system's pci.ids when it has the device, else the
// vendor (when known) and the device id.
func gpuName(fsys fs.FS, vendor, device string) string {
	vname := knownVendors[vendor]
	if vname == "" {
		vname = "vendor " + vendor
	}
	for _, path := range []string{"usr/share/hwdata/pci.ids", "usr/share/misc/pci.ids", "usr/share/pci.ids"} {
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			continue
		}
		if model := lookupPCIDevice(data, vendor, device); model != "" {
			return vname + " " + model
		}
		break
	}
	if device != "" {
		return vname + " device " + device
	}
	return vname
}

// lookupPCIDevice finds a device's name in pci.ids text: vendor lines start at the left
// margin ("10de  NVIDIA Corporation"), their devices one tab in ("\t2484  GA104 [GeForce
// RTX 3070]"). The bracketed marketing name is preferred when there is one.
func lookupPCIDevice(data []byte, vendor, device string) string {
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	inVendor := false
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line[0] != '\t' {
			if inVendor {
				return "" // the vendor's block ended without the device
			}
			id, _, _ := strings.Cut(line, " ")
			inVendor = strings.EqualFold(id, vendor)
			continue
		}
		if !inVendor || strings.HasPrefix(line, "\t\t") {
			continue
		}
		id, name, ok := strings.Cut(strings.TrimPrefix(line, "\t"), " ")
		if !ok || !strings.EqualFold(id, device) {
			continue
		}
		name = strings.TrimSpace(name)
		if open := strings.LastIndex(name, "["); open >= 0 && strings.HasSuffix(name, "]") {
			return name[open+1 : len(name)-1]
		}
		return name
	}
	return ""
}
