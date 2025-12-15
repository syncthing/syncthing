//go:build !android

package discover

import (
	"time"
)

const (
	BroadcastInterval = 30 * time.Second
)
