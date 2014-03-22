// Package martini is a powerful package for quickly writing modular web applications/services in Golang.
//
// For a full guide visit http://github.com/codegangsta/martini
//
//  package main
//
//  import "github.com/codegangsta/martini"
//
//  func main() {
//    m := martini.Classic()
//
//    m.Get("/", func() string {
//      return "Hello world!"
//    })
//
//    m.Run()
//  }
package martini

import (
	"github.com/codegangsta/inject"
	"log"
	"net/http"
	"os"
	"reflect"
)

// Martini represents the top level web application. inject.Injector methods can be invoked to map services on a global level.
type Martini struct {
	inject.Injector
	handlers []Handler
	action   Handler
	logger   *log.Logger
}

// New creates a bare bones Martini instance. Use this method if you want to have full control over the middleware that is used.
func New() *Martini {
	m := &Martini{inject.New(), []Handler{}, func() {}, log.New(os.Stdout, "[martini] ", 0)}
	m.Map(m.logger)
	return m
}

// Use adds a middleware Handler to the stack. Will panic if the handler is not a callable func. Middleware Handlers are invoked in the order that they are added.
func (m *Martini) Use(handler Handler) {
	validateHandler(handler)

	m.handlers = append(m.handlers, handler)
}

// ServeHTTP is the HTTP Entry point for a Martini instance. Useful if you want to control your own HTTP server.
func (m *Martini) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	m.createContext(res, req).run()
}

// Action sets the handler that will be called after all the middleware has been invoked. This is set to martini.Router in a martini.Classic().
func (m *Martini) Action(handler Handler) {
	validateHandler(handler)
	m.action = handler
}

// Run the http server. Listening on os.GetEnv("PORT") or 3000 by default.
func (m *Martini) Run() {
	port := os.Getenv("PORT")
	if len(port) == 0 {
		port = "3000"
	}

	m.logger.Println("listening on port " + port)
	m.logger.Fatalln(http.ListenAndServe(":"+port, m))
}

// Handlers sets the entire middleware stack with the given Handlers. This will clear any current middleware handlers.
// Will panic if any of the handlers is not a callable function
func (m *Martini) Handlers(handlers ...Handler) {
	m.handlers = make([]Handler, 0)
	for _, handler := range handlers {
		m.Use(handler)
	}
}

func (m *Martini) createContext(res http.ResponseWriter, req *http.Request) *context {
	c := &context{inject.New(), append(m.handlers, m.action), NewResponseWriter(res), 0}
	c.SetParent(m)
	c.MapTo(c, (*Context)(nil))
	c.MapTo(c.rw, (*http.ResponseWriter)(nil))
	c.Map(req)
	return c
}

// ClassicMartini represents a Martini with some reasonable defaults. Embeds the router functions for convenience.
type ClassicMartini struct {
	*Martini
	Router
}

// Classic creates a classic Martini with some basic default middleware - martini.Logger, martini.Recovery, and martini.Static.
func Classic() *ClassicMartini {
	r := NewRouter()
	m := New()
	m.Use(Logger())
	m.Use(Recovery())
	m.Use(Static("public"))
	m.Action(r.Handle)
	return &ClassicMartini{m, r}
}

// Handler can be any callable function. Martini attempts to inject services into the handler's argument list.
// Martini will panic if an argument could not be fullfilled via dependency injection.
type Handler interface{}

func validateHandler(handler Handler) {
	if reflect.TypeOf(handler).Kind() != reflect.Func {
		panic("martini handler must be a callable func")
	}
}

// Context represents a request context. Services can be mapped on the request level from this interface.
type Context interface {
	inject.Injector
	// Next is an optional function that Middleware Handlers can call to yield the until after
	// the other Handlers have been executed. This works really well for any operations that must
	// happen after an http request
	Next()
	written() bool
}

type context struct {
	inject.Injector
	handlers []Handler
	rw       ResponseWriter
	index    int
}

func (c *context) Next() {
	c.index += 1
	c.run()
}

func (c *context) written() bool {
	return c.rw.Written()
}

func (c *context) run() {
	for c.index < len(c.handlers) {
		_, err := c.Invoke(c.handlers[c.index])
		if err != nil {
			panic(err)
		}
		c.index += 1

		if c.rw.Written() {
			return
		}
	}
}
