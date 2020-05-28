function build {
    go run build.go @args
}

$cmd, $rest = $args
switch ($cmd) {
    "test" {
        $env:LOGGER_DISCARD=1
        build test
    }

    "bench" {
        $env:LOGGER_DISCARD=1
        build bench
    }

    default {
        build @rest
    }
}
