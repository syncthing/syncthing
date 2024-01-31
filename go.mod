module github.com/syncthing/syncthing

go 1.20

require (
	github.com/AudriusButkevicius/recli v0.0.7-0.20220911121932-d000ce8fbf0f
	github.com/alecthomas/kong v0.8.1
	github.com/calmh/incontainer v1.0.0
	github.com/calmh/xdr v1.1.0
	github.com/ccding/go-stun v0.1.4
	github.com/chmduquesne/rollinghash v4.0.0+incompatible
	github.com/d4l3k/messagediff v1.2.1
	github.com/flynn-archive/go-shlex v0.0.0-20150515145356-3f9db97f8568
	github.com/getsentry/raven-go v0.2.0
	github.com/go-ldap/ldap/v3 v3.4.6
	github.com/gobwas/glob v0.2.3
	github.com/gogo/protobuf v1.3.2
	github.com/greatroar/blobloom v0.7.2
	github.com/hashicorp/golang-lru/v2 v2.0.7
	github.com/jackpal/gateway v1.0.10
	github.com/jackpal/go-nat-pmp v1.0.2
	github.com/julienschmidt/httprouter v1.3.0
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/lib/pq v1.10.9
	github.com/maruel/panicparse/v2 v2.3.1
	github.com/maxbrunsfeld/counterfeiter/v6 v6.8.1
	github.com/minio/sha256-simd v1.0.1
	github.com/miscreant/miscreant.go v0.0.0-20200214223636-26d376326b75
	github.com/oschwald/geoip2-golang v1.9.0
	github.com/pierrec/lz4/v4 v4.1.21
	github.com/prometheus/client_golang v1.18.0
	github.com/quic-go/quic-go v0.40.1
	github.com/rcrowley/go-metrics v0.0.0-20201227073835-cf1acfcdf475
	github.com/shirou/gopsutil/v3 v3.23.11
	github.com/syncthing/notify v0.0.0-20210616190510-c6b7342338d2
	github.com/syndtr/goleveldb v1.0.1-0.20220721030215-126854af5e6d
	github.com/thejerf/suture/v4 v4.0.2
	github.com/urfave/cli v1.22.14
	github.com/vitrun/qart v0.0.0-20160531060029-bf64b92db6b0
	github.com/willabides/kongplete v0.4.0
	golang.org/x/crypto v0.18.0
	golang.org/x/exp v0.0.0-20240119083558-1b970713d09a
	golang.org/x/net v0.20.0
	golang.org/x/sys v0.16.0
	golang.org/x/text v0.14.0
	golang.org/x/time v0.5.0
	golang.org/x/tools v0.17.0
	google.golang.org/protobuf v1.32.0
)

require (
	github.com/Azure/go-ntlmssp v0.0.0-20221128193559-754e69321358 // indirect
	github.com/alecthomas/assert/v2 v2.5.0 // indirect
	github.com/alecthomas/repr v0.3.0 // indirect
	github.com/alexbrainman/sspi v0.0.0-20231016080023-1a75b4708caa // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/certifi/gocertifi v0.0.0-20210507211836-431795d63e8d // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.3 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-asn1-ber/asn1-ber v1.5.5 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/pprof v0.0.0-20240130152714-0ed6a68c8d9e // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/klauspost/cpuid/v2 v2.2.6 // indirect
	github.com/nxadm/tail v1.4.11 // indirect
	github.com/onsi/ginkgo/v2 v2.15.0 // indirect
	github.com/onsi/gomega v1.31.1 // indirect
	github.com/oschwald/maxminddb-golang v1.12.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/posener/complete v1.2.3 // indirect
	github.com/power-devops/perfstat v0.0.0-20221212215047-62379fc7944b // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.46.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/quic-go/qtls-go1-20 v0.4.1 // indirect
	github.com/riywo/loginshell v0.0.0-20200815045211-7d26008be1ab // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.uber.org/mock v0.4.0 // indirect
	golang.org/x/mod v0.14.0 // indirect
)

// https://github.com/gobwas/glob/pull/55
replace github.com/gobwas/glob v0.2.3 => github.com/calmh/glob v0.0.0-20220615080505-1d823af5017b
