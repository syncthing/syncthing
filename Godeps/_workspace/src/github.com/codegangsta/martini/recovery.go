package martini

import (
	"log"
	"net/http"
	"runtime/debug"
)

// Recovery returns a middleware that recovers from any panics and writes a 500 if there was one.
func Recovery() Handler {
	return func(res http.ResponseWriter, c Context, logger *log.Logger) {
		defer func() {
			if err := recover(); err != nil {
				res.WriteHeader(http.StatusInternalServerError)
				logger.Printf("PANIC: %s\n%s", err, debug.Stack())
			}
		}()

		c.Next()
	}
}
