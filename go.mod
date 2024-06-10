module github.com/syncthing/syncthing

go 1.21.0

require (
	github.com/AudriusButkevicius/recli v0.0.7-0.20220911121932-d000ce8fbf0f
	github.com/alecthomas/kong v0.9.0
	github.com/calmh/incontainer v1.0.0
	github.com/calmh/xdr v1.1.0
	github.com/ccding/go-stun v0.1.4
	github.com/chmduquesne/rollinghash v4.0.0+incompatible
	github.com/d4l3k/messagediff v1.2.1
	github.com/flynn-archive/go-shlex v0.0.0-20150515145356-3f9db97f8568
	github.com/getsentry/raven-go v0.2.0
	github.com/go-ldap/ldap/v3 v3.4.8
	github.com/gobwas/glob v0.2.3
	github.com/gogo/protobuf v1.3.2
	github.com/greatroar/blobloom v0.8.0
	github.com/hashicorp/golang-lru/v2 v2.0.7
	github.com/jackpal/gateway v1.0.15
	github.com/jackpal/go-nat-pmp v1.0.2
	github.com/julienschmidt/httprouter v1.3.0
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/lib/pq v1.10.9
	github.com/maruel/panicparse/v2 v2.3.1
	github.com/maxbrunsfeld/counterfeiter/v6 v6.8.1
	github.com/maxmind/geoipupdate/v6 v6.1.0
	github.com/minio/sha256-simd v1.0.1
	github.com/miscreant/miscreant.go v0.0.0-20200214223636-26d376326b75
	github.com/oschwald/geoip2-golang v1.11.0
	github.com/pierrec/lz4/v4 v4.1.21
	github.com/prometheus/client_golang v1.19.1
	github.com/quic-go/quic-go v0.45.0
	github.com/rabbitmq/amqp091-go v1.10.0
	github.com/rcrowley/go-metrics v0.0.0-20201227073835-cf1acfcdf475
	github.com/shirou/gopsutil/v3 v3.24.5
	github.com/syncthing/notify v0.0.0-20210616190510-c6b7342338d2
	github.com/syndtr/goleveldb v1.0.1-0.20220721030215-126854af5e6d
	github.com/thejerf/suture/v4 v4.0.5
	github.com/urfave/cli v1.22.15
	github.com/vitrun/qart v0.0.0-20160531060029-bf64b92db6b0
	github.com/willabides/kongplete v0.4.0
	go.uber.org/automaxprocs v1.5.3
	golang.org/x/crypto v0.23.0
	golang.org/x/net v0.25.0
	golang.org/x/sys v0.20.0
	golang.org/x/text v0.15.0
	golang.org/x/time v0.5.0
	golang.org/x/tools v0.21.0
	google.golang.org/protobuf v1.34.1
)

require (
	github.com/Azure/go-ntlmssp v0.0.0-20221128193559-754e69321358 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/certifi/gocertifi v0.0.0-20210507211836-431795d63e8d // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.4 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-asn1-ber/asn1-ber v1.5.7 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/gofrs/flock v0.8.1 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/pprof v0.0.0-20240528025155-186aa0362fba // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/klauspost/cpuid/v2 v2.2.7 // indirect
	github.com/nxadm/tail v1.4.11 // indirect
	github.com/onsi/ginkgo/v2 v2.19.0 // indirect
	github.com/oschwald/maxminddb-golang v1.13.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/posener/complete v1.2.3 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.54.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/riywo/loginshell v0.0.0-20200815045211-7d26008be1ab // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/stretchr/testify v1.9.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.uber.org/mock v0.4.0 // indirect
	golang.org/x/exp v0.0.0-20240531132922-fd00a4e0eefc // indirect
	golang.org/x/mod v0.17.0 // indirect
	golang.org/x/sync v0.7.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// https://github.com/gobwas/glob/pull/55
replace github.com/gobwas/glob v0.2.3 => github.com/calmh/glob v0.0.0-20220615080505-1d823af5017b
