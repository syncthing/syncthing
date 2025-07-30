package slogutil

import "log/slog"

func Error(err error) slog.Attr {
	return slog.Any("error", err)
}

func Address(v any) slog.Attr {
	return slog.Any("address", v)
}

func FilePath(path string) slog.Attr {
	return slog.String("path", path)
}
