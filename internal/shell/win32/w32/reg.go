//go:build windows

package w32

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

// RegDWORD reads the DWORD (or QWORD) value name of the HKEY_CURRENT_USER
// key path. A missing key or value is an error wrapping
// registry.ErrNotExist (windows.ERROR_FILE_NOT_FOUND).
func RegDWORD(path, name string) (uint32, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if err != nil {
		return 0, fmt.Errorf(`w32: RegOpenKeyEx(HKCU\%s): %w`, path, err)
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue(name)
	if err != nil {
		return 0, fmt.Errorf(`w32: RegQueryValueEx(HKCU\%s, %s): %w`, path, name, err)
	}
	return uint32(v), nil
}
