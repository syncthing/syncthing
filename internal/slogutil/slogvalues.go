package slogutil

import (
	"log/slog"
)

func Address(v any) slog.Attr {
	return slog.Any("address", v)
}

func Device(v any) slog.Attr {
	return slog.Any("device", v)
}

func Error(err any) slog.Attr {
	if err == nil {
		return slog.Attr{}
	}
	return slog.Any("error", err)
}

func FilePath(path string) slog.Attr {
	return slog.String("path", path)
}

func URI(v any) slog.Attr {
	return slog.Any("uri", v)
}
