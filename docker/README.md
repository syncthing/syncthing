# Syncthing Docker

## Build Args

### Supported

|BUILD_GOOS|BUILD_GOARCH|SERVE_U|SERVE_R|QEMUARCH|
|-|-|-|-|-|
|linux|386|i386|alpine|i386|
|linux|amd64|amd64|alpine|amd64|
|linux|arm|arm32v6|alpine|arm|
|linux|arm64|arm64v8|alpine|aarch64|
|linux|ppc64le|ppc64le|alpine|ppc64le|
|linux|s390x|s390x|alpine|s390x|

### Unsupported (no base image available)

|BUILD_GOOS|BUILD_GOARCH|SERVE_U|SERVE_R|QEMUARCH|
|-|-|-|-|-|
|linux|mips|!|!|mips|
|linux|mips64|!|!|mips64|
|linux|mips64le|!|!|mips64el|
|linux|mipsle|!|!|mipsel|
|linux|ppc64|!|!|ppc64|
