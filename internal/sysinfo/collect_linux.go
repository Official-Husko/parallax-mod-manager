package sysinfo

import "os"

func collect() Info {
	return collectLinux(os.DirFS("/"), os.Getenv)
}
