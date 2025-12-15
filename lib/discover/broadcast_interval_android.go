//go:build android

package discover

import (
	"time"
)

// Allow mobile devices to sleep between broadcasts in order to save battery
const (
	BroadcastInterval = 120 * time.Second
)
