package main

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/go-ble/ble"
	ibbq "github.com/lukeryannetnz/go-ibbq-mqtt/internal/ibbq"
)

// ScanForDevices scans for all connectable iBBQ advertisements for the given duration.
func ScanForDevices(ctx context.Context, duration time.Duration) ([]string, error) {
	seen := make(map[string]bool)
	var mu sync.Mutex

	scanCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	err := ble.Scan(scanCtx, false,
		func(a ble.Advertisement) {
			mac := normalizeMAC(a.Addr().String())
			mu.Lock()
			seen[mac] = true
			mu.Unlock()
		},
		func(a ble.Advertisement) bool {
			return ibbq.IsSupportedDeviceName(a.LocalName()) && a.Connectable()
		},
	)

	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}

	macs := make([]string, 0, len(seen))
	for mac := range seen {
		macs = append(macs, mac)
	}
	sort.Strings(macs)
	return macs, nil
}
