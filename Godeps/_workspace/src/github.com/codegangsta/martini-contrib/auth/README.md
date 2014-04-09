# auth
Martini middleware/handler for http basic authentication.

[API Reference](http://godoc.org/github.com/codegangsta/martini-contrib/auth)

## Usage

~~~ go
import (
  "github.com/codegangsta/martini"
  "github.com/codegangsta/martini-contrib/auth"
)

func main() {
  m := martini.Classic()
  // authenticate every request
  m.Use(auth.Basic("username", "secretpassword"))
  m.Run()
}

~~~

## Authors
* [Jeremy Saenz](http://github.com/codegangsta)
* [Brendon Murphy](http://github.com/bemurphy)
