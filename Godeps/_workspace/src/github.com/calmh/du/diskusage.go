package du

// Usage holds information about total and available storage on a volume.
type Usage struct {
	TotalBytes int64 // Size of volume
	FreeBytes  int64 // Unused size
	AvailBytes int64 // Available to a non-privileged user
}
