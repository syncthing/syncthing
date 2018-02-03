### Extensions to the "os" package.

[![GoDoc](https://godoc.org/github.com/kardianos/osext?status.svg)](https://godoc.org/github.com/kardianos/osext)

## Find the current Executable and ExecutableFolder.

As of go1.8 the Executable function may be found in `os`. The Executable function
in the std lib `os` package is used if available.

There is sometimes utility in finding the current executable file
that is running. This can be used for upgrading the current executable
or finding resources located relative to the executable file. Both
working directory and the os.Args[0] value are arbitrary and cannot
be relied on; os.Args[0] can be "faked".

Multi-platform and supports:
 * Linux
 * OS X
 * Windows
 * Plan 9
 * BSDs.
