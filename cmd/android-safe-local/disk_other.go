//go:build !windows

package main

import "errors"

func diskFreeGBOS(_ string) (float64, error) {
	return 0, errors.New("disk free space not supported on this OS")
}
