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
	return slog.Any("error", err)
}

func FilePath(path string) slog.Attr {
	return slog.String("path", path)
}

func Folder(id, label, typ string) slog.Attr {
	if label == "" || label == id {
		return slog.Group("folder", slog.String("id", id), slog.String("type", typ))
	}
	return slog.Group("folder", slog.String("label", label), slog.String("id", id), slog.String("type", typ))
}

func URI(v any) slog.Attr {
	return slog.Any("uri", v)
}
