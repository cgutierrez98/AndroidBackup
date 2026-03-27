//go:build windows

package main

import (
	"path/filepath"
	"syscall"
	"unsafe"
)

func diskFreeGBOS(path string) (float64, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return 0, err
	}
	ptr, err := syscall.UTF16PtrFromString(abs)
	if err != nil {
		return 0, err
	}
	var freeBytes, totalBytes, totalFreeBytes uint64
	r1, _, callErr := syscall.NewLazyDLL("kernel32.dll").
		NewProc("GetDiskFreeSpaceExW").
		Call(
			uintptr(unsafe.Pointer(ptr)),
			uintptr(unsafe.Pointer(&freeBytes)),
			uintptr(unsafe.Pointer(&totalBytes)),
			uintptr(unsafe.Pointer(&totalFreeBytes)),
		)
	if r1 == 0 {
		return 0, callErr
	}
	return float64(freeBytes) / 1e9, nil
}
