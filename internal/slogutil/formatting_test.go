package slogutil

import (
	"log/slog"
	"testing"
)

func TestFormattingHandler(t *testing.T) {
	h := &formattingHandler{}
	l := slog.New(h).With("a", "a")
	l.Info("Message here", "attr1", "val1", "attr2", 2)
	l.Info("Message here", "attr1", "val1", slog.Group("foo", "attr2", 2, slog.Group("bar", "attr3", "3")))
	l2 := l.WithGroup("foo")
	l2.Info("Message here", "attr1", "val1", "attr2", 2)
	l3 := l2.WithGroup("bar")
	l3.Info("Message here", "attr1", "val1", "attr2", 2)
}
