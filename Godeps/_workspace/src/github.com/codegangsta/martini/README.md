# Martini [![Build Status](https://drone.io/github.com/codegangsta/martini/status.png)](https://drone.io/github.com/codegangsta/martini/latest) [![GoDoc](https://godoc.org/github.com/codegangsta/martini?status.png)](http://godoc.org/github.com/codegangsta/martini)

Martini is a powerful package for quickly writing modular web applications/services in Golang.

## Getting Started

After installing Go and setting up your [GOPATH](http://golang.org/doc/code.html#GOPATH), create your first `.go` file. We'll call it `server.go`.

~~~ go
package main

import "github.com/codegangsta/martini"

func main() {
  m := martini.Classic()
  m.Get("/", func() string {
    return "Hello world!"
  })
  m.Run()
}
~~~

Then install the Martini package (**go 1.1** and greater is required):
~~~
go get github.com/codegangsta/martini
~~~

Then run your server:
~~~
go run server.go
~~~

You will now have a Martini webserver running on `localhost:3000`.

## Getting Help

Join the [Mailing list](https://groups.google.com/forum/#!forum/martini-go)

Watch the [Demo Video](http://martini.codegangsta.io/#demo)

## Features
* Extremely simple to use.
* Non-intrusive design.
* Play nice with other Golang packages.
* Awesome path matching and routing.
* Modular design - Easy to add functionality, easy to rip stuff out.
* Lots of good handlers/middlewares to use.
* Great 'out of the box' feature set.
* **Fully compatible with the [http.HandlerFunc](http://godoc.org/net/http#HandlerFunc) interface.**

## More Middleware
For more middleware and functionality, check out the [martini-contrib](http://github.com/codegangsta/martini-contrib) repository.

## Table of Contents
* [Classic Martini](#classic-martini)
  * [Handlers](#handlers)
  * [Routing](#routing)
  * [Services](#services)
  * [Serving Static Files](#serving-static-files)
* [Middleware Handlers](#middleware-handlers)
  * [Next()](#next)
* [FAQ](#faq)

## Classic Martini
To get up and running quickly, [martini.Classic()](http://godoc.org/github.com/codegangsta/martini#Classic) provides some reasonable defaults that work well for most web applications:
~~~ go
  m := martini.Classic()
  // ... middleware and routing goes here
  m.Run()
~~~

Below is some of the functionality [martini.Classic()](http://godoc.org/github.com/codegangsta/martini#Classic) pulls in automatically:
  * Request/Response Logging - [martini.Logger](http://godoc.org/github.com/codegangsta/martini#Logger)
  * Panic Recovery - [martini.Recovery](http://godoc.org/github.com/codegangsta/martini#Recovery)
  * Static File serving - [martini.Static](http://godoc.org/github.com/codegangsta/martini#Static)
  * Routing - [martini.Router](http://godoc.org/github.com/codegangsta/martini#Router)

### Handlers
Handlers are the heart and soul of Martini. A handler is basically any kind of callable function:
~~~ go
m.Get("/", func() {
  println("hello world")
})
~~~

#### Return Values
If a handler returns something, Martini will write the result to the current [http.ResponseWriter](http://godoc.org/net/http#ResponseWriter) as a string:
~~~ go
m.Get("/", func() string {
  return "hello world" // HTTP 200 : "hello world"
})
~~~

You can also optionally return a status code:
~~~ go
m.Get("/", func() (int, string) {
  return 418, "i'm a teapot" // HTTP 418 : "i'm a teapot"
})
~~~

#### Service Injection
Handlers are invoked via reflection. Martini makes use of *Dependency Injection* to resolve dependencies in a Handlers argument list. **This makes Martini completely  compatible with golang's `http.HandlerFunc` interface.** 

If you add an argument to your Handler, Martini will search it's list of services and attempt to resolve the dependency via type assertion:
~~~ go
m.Get("/", func(res http.ResponseWriter, req *http.Request) { // res and req are injected by Martini
  res.WriteHeader(200) // HTTP 200
})
~~~

The following services are included with [martini.Classic()](http://godoc.org/github.com/codegangsta/martini#Classic):
  * [*log.Logger](http://godoc.org/log#Logger) - Global logger for Martini.
  * [martini.Context](http://godoc.org/github.com/codegangsta/martini#Context) - http request context.
  * [martini.Params](http://godoc.org/github.com/codegangsta/martini#Params) - `map[string]string` of named params found by route matching.
  * [martini.Routes](http://godoc.org/github.com/codegangsta/martini#Routes) - Route helper service.
  * [http.ResponseWriter](http://godoc.org/net/http/#ResponseWriter) - http Response writer interface.
  * [*http.Request](http://godoc.org/net/http/#Request) - http Request.

### Routing
In Martini, a route is an HTTP method paired with a URL-matching pattern.
Each route can take one or more handler methods:
~~~ go
m.Get("/", func() {
  // show something
})

m.Patch("/", func() {
  // update something
})

m.Post("/", func() {
  // create something
})

m.Put("/", func() {
  // replace something
})

m.Delete("/", func() {
  // destroy something
})

m.Options("/", func() {
  // http options
})

m.NotFound(func() {
  // handle 404
})
~~~

Routes are matched in the order they are defined. The first route that
matches the request is invoked.

Route patterns may include named parameters, accessible via the [martini.Params](http://godoc.org/github.com/codegangsta/martini#Params) service:
~~~ go
m.Get("/hello/:name", func(params martini.Params) string {
  return "Hello " + params["name"]
})
~~~

Routes can be matched with regular expressions and globs as well:
~~~ go
m.Get("/hello/**", func(params martini.Params) string {
  return "Hello " + params["_1"]
})
~~~

Route handlers can be stacked on top of each other, which is useful for things like authentication and authorization:
~~~ go
m.Get("/secret", authorize, func() {
  // this will execute as long as authorize doesn't write a response
})
~~~

### Services
Services are objects that are available to be injected into a Handler's argument list. You can map a service on a *Global* or *Request* level.

#### Global Mapping
A Martini instance implements the inject.Injector interface, so mapping a service is easy:
~~~ go
db := &MyDatabase{}
m := martini.Classic()
m.Map(db) // the service will be available to all handlers as *MyDatabase
// ...
m.Run()
~~~

#### Request-Level Mapping
Mapping on the request level can be done in a handler via [martini.Context](http://godoc.org/github.com/codegangsta/martini#Context):
~~~ go
func MyCustomLoggerHandler(c martini.Context, req *http.Request) {
  logger := &MyCustomLogger{req}
  c.Map(logger) // mapped as *MyCustomLogger
}
~~~

#### Mapping values to Interfaces
One of the most powerful parts about services is the ability to map a service to an interface. For instance, if you wanted to override the [http.ResponseWriter](http://godoc.org/net/http#ResponseWriter) with an object that wrapped it and performed extra operations, you can write the following handler:
~~~ go
func WrapResponseWriter(res http.ResponseWriter, c martini.Context) {
  rw := NewSpecialResponseWriter(res)
  c.MapTo(rw, (*http.ResponseWriter)(nil)) // override ResponseWriter with our wrapper ResponseWriter
}
~~~

### Serving Static Files
A [martini.Classic()](http://godoc.org/github.com/codegangsta/martini#Classic) instance automatically serves static files from the "public" directory in the root of your server.
You can serve from more directories by adding more [martini.Static](http://godoc.org/github.com/codegangsta/martini#Static) handlers.
~~~ go
m.Use(martini.Static("assets")) // serve from the "assets" directory as well
~~~

## Middleware Handlers
Middleware Handlers sit between the incoming http request and the router. In essence they are no different than any other Handler in Martini. You can add a middleware handler to the stack like so:
~~~ go
m.Use(func() {
  // do some middleware stuff
})
~~~

You can have full control over the middleware stack with the `Handlers` function:
~~~ go
m.Handlers(
  Middleware1,
  Middleware2,
  Middleware3,
)
~~~

Middleware Handlers work really well for things like logging, authorization, authentication, sessions, gzipping, error pages and any other operations that must happen before or after an http request:
~~~ go
// validate an api key
m.Use(func(res http.ResponseWriter, req *http.Request) {
  if req.Header.Get("X-API-KEY") != "secret123" {
    res.WriteHeader(http.StatusUnauthorized)
  }
})
~~~

### Next()
[Context.Next()](http://godoc.org/github.com/codegangsta/martini#Context) is an optional function that Middleware Handlers can call to yield the until after the other Handlers have been executed. This works really well for any operations that must happen after an http request:
~~~ go
// log before and after a request
m.Use(func(c martini.Context, log *log.Logger){
  log.Println("before a request")

  c.Next()
  
  log.Println("after a request")
})
~~~

## FAQ

### Where do I find middleware X?

Start by looking in the [martini-contrib](http://github.com/codegangsta/martini-contrib) package. If it is not there feel free to put up a Pull Request for one.

* [auth](https://github.com/codegangsta/martini-contrib/tree/master/auth) - Handlers for authentication.
* [form](https://github.com/codegangsta/martini-contrib/tree/master/form) - Handler for parsing and mapping form fields.
* [gzip](https://github.com/codegangsta/martini-contrib/tree/master/gzip) - Handler for adding gzip compress to requests
* [render](https://github.com/codegangsta/martini-contrib/tree/master/render) - Handler that provides a service for easily rendering JSON and HTML templates.
* [acceptlang](https://github.com/codegangsta/martini-contrib/tree/master/acceptlang) - Handler for parsing the `Accept-Language` HTTP header.

### How do I integrate with existing servers?

A Martini instance implements `http.Handler`, so it can easily be used to serve subtrees 
on existing Go servers. For example this is a working Martini app for Google App Engine:

~~~ go
package hello

import (
  "net/http"
  "github.com/codegangsta/martini"
)

func init() {
  m := martini.Classic()
  m.Get("/", func() string {
    return "Hello world!"
  })
  http.Handle("/", m)
}
~~~

### How do I change the port/host?

Martini's `Run` function looks for the PORT environment variable and uses that. Otherwise Martini will default to port 3000.
To have more flexibility over port and host, use the `http.ListenAndServe` function instead.

~~~ go
  m := martini.Classic()
  // ...
  http.ListenAndServe(":8080", m)
~~~

## Contributing
Martini is meant to be kept tiny and clean. Most contributions should end up in the [martini-contrib](http://github.com/codegangsta/martini-contrib) repository. If you do have a contribution for the core of Martini feel free to put up a Pull Request.

## About
Martini is obsessively designed by none other than the [Code Gangsta](http://codegangsta.io/)
