ini [![Build Status](https://drone.io/github.com/calmh/ini/status.png)](https://drone.io/github.com/calmh/ini/latest)
===

Yet another .INI file parser / writer. Created because the existing ones
were either not general enough (allowing easy access to all parts of the
original file) or made annoying assumptions about the format. And
probably equal parts NIH. You might want to just write your own instead
of using this one, you know that's where you'll end up in the end
anyhow.

Documentation
-------------

http://godoc.org/github.com/calmh/ini

Example
-------

```go
fd, _ := os.Open("foo.ini")
cfg := ini.Parse(fd)
fd.Close()

val := cfg.Get("general", "foo")
cfg.Set("general", "bar", "baz")

fd, _ = os.Create("bar.ini")
err := cfg.Write(fd)
if err != nil {
	// ...
}
err = fd.Close()

```

License
-------

MIT
