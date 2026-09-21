package sysinfo

import (
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

const pciIDs = `# fake pci.ids
1002  Advanced Micro Devices, Inc. [AMD/ATI]
	73bf  Navi 21 [Radeon RX 6800/6800 XT / 6900 XT]
		1002 0e3a  Some subsystem
10de  NVIDIA Corporation
	2484  GA104 [GeForce RTX 3070]
	2504  GA106 [GeForce RTX 3060 Lite Hash Rate]
	1f08  TU106 [GeForce RTX 2060]
8086  Intel Corporation
	9a49  TigerLake-LP GT2 [Iris Xe Graphics]
`

// fakeMachine is a Linux box: an NVIDIA card in one slot, an Intel one in another.
func fakeMachine() fstest.MapFS {
	return fstest.MapFS{
		"etc/os-release":                         {Data: []byte("NAME=\"EndeavourOS\"\nPRETTY_NAME=\"EndeavourOS\"\nID=endeavouros\nHOSTNAME=must-not-be-read\n")},
		"proc/sys/kernel/osrelease":              {Data: []byte("6.18.49-2-lts\n")},
		"proc/cpuinfo":                           {Data: []byte("processor\t: 0\nmodel name\t: AMD Ryzen 9  7950X 16-Core Processor\ncpu MHz\t\t: 4500\n\nprocessor\t: 1\nmodel name\t: AMD Ryzen 9  7950X 16-Core Processor\n\n")},
		"proc/meminfo":                           {Data: []byte("MemTotal:       32768000 kB\nMemFree:         1000 kB\nMemAvailable:   16384000 kB\n")},
		"usr/lib/pkgconfig/webkit2gtk-4.1.pc":    {Data: []byte("prefix=/usr\nName: WebKit\nVersion: 2.50.1\n")},
		"proc/sys/fs/inotify/max_user_watches":   {Data: []byte("524288\n")},
		"proc/sys/fs/inotify/max_user_instances": {Data: []byte("128\n")},
		"usr/share/hwdata/pci.ids":               {Data: []byte(pciIDs)},
		"sys/class/drm/card0/device/vendor":      {Data: []byte("0x10de\n")},
		"sys/class/drm/card0/device/device":      {Data: []byte("0x2484\n")},
		"sys/class/drm/card0/device/uevent":      {Data: []byte("DRIVER=nvidia\nPCI_ID=10DE:2484\n")},
		"proc/driver/nvidia/version":             {Data: []byte("NVRM version: NVIDIA UNIX Open Kernel Module for x86_64  595.58.03  Release Build  (archlinux-builder@)\nGCC version: gcc 15\n")},
		"sys/class/drm/card1/device/vendor":      {Data: []byte("0x8086\n")},
		"sys/class/drm/card1/device/device":      {Data: []byte("0x9a49\n")},
		"sys/class/drm/card1/device/uevent":      {Data: []byte("DRIVER=i915\n")},
		"sys/class/drm/card1-HDMI-A-1/status":    {Data: []byte("connected\n")}, // a connector, not an adapter
	}
}

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestLinuxReadsAWholeMachine(t *testing.T) {
	info := collectLinux(fakeMachine(), env(map[string]string{"XDG_SESSION_TYPE": "wayland", "XDG_CURRENT_DESKTOP": "KDE"}))
	want := []string{
		"OS: EndeavourOS (Linux 6.18.49-2-lts)",
		"Session: Wayland, KDE",
		"Webview: WebKitGTK 2.50.1",
		"CPU: AMD Ryzen 9 7950X 16-Core Processor, 2 threads",
		"Memory: 31.2 GiB total, 15.6 GiB available",
		"GPU: NVIDIA GeForce RTX 3070, driver nvidia 595.58.03",
		"GPU: Intel Iris Xe Graphics, driver i915",
		"Limits: inotify watches 524288, inotify instances 128",
	}
	got := info.Lines()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("lines =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestLinuxLeavesOutWhatItCannotFindAndNeverFails(t *testing.T) {
	info := collectLinux(fstest.MapFS{}, env(nil))
	if lines := info.Lines(); len(lines) != 0 {
		t.Errorf("an empty machine gave lines %v", lines)
	}
	// A card whose id is not in pci.ids is still listed, with what is known.
	m := fstest.MapFS{
		"sys/class/drm/card0/device/vendor": {Data: []byte("0x10de\n")},
		"sys/class/drm/card0/device/device": {Data: []byte("0xffff\n")},
	}
	got := collectLinux(m, env(nil)).GPUs
	if len(got) != 1 || got[0].Name != "NVIDIA device ffff" {
		t.Errorf("GPUs = %+v", got)
	}
	// Garbage in every file is survived.
	junk := fstest.MapFS{
		"proc/cpuinfo": {Data: []byte("\x00\x01 nonsense")}, "proc/meminfo": {Data: []byte("MemTotal: lots kB")},
		"etc/os-release": {Data: []byte("===")}, "usr/share/hwdata/pci.ids": {Data: []byte("\xff\xfe")},
	}
	_ = collectLinux(junk, env(nil)).Lines()
}

func TestLinuxSessionFallbacks(t *testing.T) {
	cases := []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"XDG_SESSION_TYPE": "x11"}, "X11"},
		{map[string]string{"XDG_SESSION_TYPE": "tty", "WAYLAND_DISPLAY": "wayland-0"}, "Wayland"},
		{map[string]string{"DISPLAY": ":0"}, "X11"},
		{map[string]string{}, ""},
	}
	for _, c := range cases {
		if got, _ := linuxSession(env(c.env)); got != c.want {
			t.Errorf("session for %v = %q, want %q", c.env, got, c.want)
		}
	}
	if linuxSandbox(env(map[string]string{"FLATPAK_ID": "x"})) != "Flatpak" || linuxSandbox(env(map[string]string{"APPIMAGE": "/x"})) != "AppImage" || linuxSandbox(env(nil)) != "" {
		t.Error("sandbox detection is wrong")
	}
}

func TestOSNameUsesNameAndVersionID(t *testing.T) {
	m := fstest.MapFS{"etc/os-release": {Data: []byte("NAME=\"Ubuntu\"\nVERSION_ID=\"24.04\"\nPRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\n")}}
	if got := collectLinux(m, env(nil)).OS; got != "Ubuntu 24.04" {
		t.Errorf("OS = %q", got)
	}
	m = fstest.MapFS{"etc/os-release": {Data: []byte("PRETTY_NAME=\"Some Linux\"\n")}}
	if got := collectLinux(m, env(nil)).OS; got != "Some Linux" {
		t.Errorf("OS = %q", got)
	}
}

func TestARMCPUInfoAndPCILookup(t *testing.T) {
	model, threads := parseCPUInfo("processor : 0\nprocessor : 1\nprocessor : 2\nprocessor : 3\nHardware : BCM2711\nModel : Raspberry Pi 4 Model B Rev 1.4\n")
	if threads != 4 || model != "BCM2711" {
		t.Errorf("model = %q, threads = %d", model, threads)
	}
	if got := lookupPCIDevice([]byte(pciIDs), "1002", "73BF"); got != "Radeon RX 6800/6800 XT / 6900 XT" {
		t.Errorf("AMD lookup = %q (the bracketed marketing name, case-insensitive ids)", got)
	}
	if got := lookupPCIDevice([]byte(pciIDs), "10de", "9999"); got != "" {
		t.Errorf("a device the vendor block lacks = %q, want empty", got)
	}
	if got := lookupPCIDevice([]byte(pciIDs), "beef", "1"); got != "" {
		t.Errorf("an unknown vendor = %q", got)
	}
}

func TestNoIdentifyingDetailsAreRead(t *testing.T) {
	m := fakeMachine()
	m["etc/hostname"] = &fstest.MapFile{Data: []byte("my-secret-hostname\n")}
	m["etc/machine-id"] = &fstest.MapFile{Data: []byte("0123456789abcdef0123456789abcdef\n")}
	all := strings.Join(collectLinux(m, env(map[string]string{"USER": "pawbeans", "HOME": "/home/pawbeans", "HOSTNAME": "my-secret-hostname"})).Lines(), "\n")
	for _, secret := range []string{"my-secret-hostname", "0123456789abcdef", "pawbeans", "must-not-be-read"} {
		if strings.Contains(all, secret) {
			t.Errorf("the report contains %q", secret)
		}
	}
}

func TestLinesSkipEmptyFieldsAndFormatSizes(t *testing.T) {
	got := Info{CPU: "X", Threads: 8, MemoryTotal: 8 << 30}.Lines()
	if strings.Join(got, "|") != "CPU: X, 8 threads|Memory: 8.0 GiB total" {
		t.Errorf("lines = %v", got)
	}
	if HumanBytes(512) != "512 B" || HumanBytes(1<<40) != "1.0 TiB" {
		t.Errorf("HumanBytes = %q / %q", HumanBytes(512), HumanBytes(1<<40))
	}
}

// The real machine: fast, and it finds at least the basics on Linux.
func TestCollectOnThisMachine(t *testing.T) {
	start := time.Now()
	info := Collect()
	if took := time.Since(start); took > 500*time.Millisecond {
		t.Errorf("Collect took %v, want it cheap enough for startup", took)
	}
	if info.Threads != runtime.NumCPU() && runtime.GOOS != "linux" {
		t.Errorf("Threads = %d", info.Threads)
	}
	if runtime.GOOS == "linux" {
		if info.MemoryTotal <= 0 || info.CPU == "" || info.Kernel == "" {
			t.Errorf("a Linux machine gave %+v", info)
		}
		t.Logf("this machine:\n  %s", strings.Join(info.Lines(), "\n  "))
	}
}
