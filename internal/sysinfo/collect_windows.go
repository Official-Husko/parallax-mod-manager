package sysinfo

import (
	"fmt"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// The Windows sources are the registry values and the memory call Windows keeps for this.
// They have not been run on Windows yet (only compiled), so each read is independent and
// a failure leaves its field empty.

// memoryStatusEx is the MEMORYSTATUSEX structure GlobalMemoryStatusEx fills in.
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

var globalMemoryStatusEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

func regString(root registry.Key, path, name string, wow64 bool) string {
	access := uint32(registry.QUERY_VALUE)
	if wow64 {
		access |= registry.WOW64_64KEY
	}
	k, err := registry.OpenKey(root, path, access)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue(name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(v)
}

func collect() Info {
	var info Info

	const nt = `SOFTWARE\Microsoft\Windows NT\CurrentVersion`
	product := regString(registry.LOCAL_MACHINE, nt, "ProductName", true)
	build := regString(registry.LOCAL_MACHINE, nt, "CurrentBuild", true)
	display := regString(registry.LOCAL_MACHINE, nt, "DisplayVersion", true)
	if display == "" {
		display = regString(registry.LOCAL_MACHINE, nt, "ReleaseId", true)
	}
	// Windows 11 still says "Windows 10" in ProductName; the build number tells them apart.
	if n, err := strconv.Atoi(build); err == nil && n >= 22000 {
		product = strings.Replace(product, "Windows 10", "Windows 11", 1)
	}
	info.OS = strings.TrimSpace(product + " " + display)
	if build != "" {
		info.Kernel = "build " + build
	}

	info.CPU = collapseSpaces(regString(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, "ProcessorNameString", true))

	var mem memoryStatusEx
	mem.Length = uint32(unsafe.Sizeof(mem))
	if r, _, _ := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&mem))); r != 0 {
		info.MemoryTotal, info.MemoryAvailable = int64(mem.TotalPhys), int64(mem.AvailPhys)
	}

	const adapters = `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}`
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, adapters, registry.ENUMERATE_SUB_KEYS|registry.WOW64_64KEY); err == nil {
		names, _ := k.ReadSubKeyNames(-1)
		k.Close()
		for _, n := range names {
			if len(n) != 4 { // the adapters are 0000, 0001 ...; "Configuration" and "Properties" are not
				continue
			}
			desc := regString(registry.LOCAL_MACHINE, adapters+`\`+n, "DriverDesc", true)
			if desc == "" {
				continue
			}
			info.GPUs = append(info.GPUs, GPU{Name: desc, Driver: regString(registry.LOCAL_MACHINE, adapters+`\`+n, "DriverVersion", true)})
		}
	}

	const webview2 = `{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`
	pv := regString(registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\`+webview2, "pv", false)
	if pv == "" {
		pv = regString(registry.CURRENT_USER, `Software\Microsoft\EdgeUpdate\Clients\`+webview2, "pv", false)
	}
	if pv != "" {
		info.Webview = fmt.Sprintf("WebView2 %s", pv)
	}
	return info
}
