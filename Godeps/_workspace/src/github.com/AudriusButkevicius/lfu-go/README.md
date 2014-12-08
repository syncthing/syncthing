A simple LFU cache for golang.  Based on the paper [An O(1) algorithm for implementing the LFU cache eviction scheme](http://dhruvbird.com/lfu.pdf).

Usage:

```go
import "github.com/dgrijalva/lfu-go"

// Make a new thing
c := lfu.New()

// Set some values
c.Set("myKey", myValue)

// Retrieve some values
myValue = c.Get("myKey")

// Evict some values
c.Evict(1)
```