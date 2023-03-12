module github.com/syncthing/syncthing

go 1.19

require (
	github.com/AudriusButkevicius/pfilter v0.0.11
	github.com/AudriusButkevicius/recli v0.0.6
	github.com/alecthomas/kong v0.7.1
	github.com/calmh/incontainer v0.0.0-20221224152218-b3e71b103d7a
	github.com/calmh/xdr v1.1.0
	github.com/ccding/go-stun v0.1.4
	github.com/certifi/gocertifi v0.0.0-20210507211836-431795d63e8d // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/chmduquesne/rollinghash v4.0.0+incompatible
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/d4l3k/messagediff v1.2.1
	github.com/flynn-archive/go-shlex v0.0.0-20150515145356-3f9db97f8568
	github.com/getsentry/raven-go v0.2.0
	github.com/go-asn1-ber/asn1-ber v1.5.4 // indirect
	github.com/go-ldap/ldap/v3 v3.4.4
	github.com/gobwas/glob v0.2.3
	github.com/gogo/protobuf v1.3.2
	github.com/golang/snappy v0.0.4 // indirect
	github.com/greatroar/blobloom v0.7.2
	github.com/hashicorp/golang-lru/v2 v2.0.1
	github.com/jackpal/gateway v1.0.7
	github.com/jackpal/go-nat-pmp v1.0.2
	github.com/julienschmidt/httprouter v1.3.0
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/klauspost/cpuid/v2 v2.2.4 // indirect
	github.com/lib/pq v1.10.7
	github.com/maruel/panicparse/v2 v2.3.1
	github.com/maxbrunsfeld/counterfeiter/v6 v6.5.0
	github.com/minio/sha256-simd v1.0.0
	github.com/miscreant/miscreant.go v0.0.0-20200214223636-26d376326b75
	github.com/oschwald/geoip2-golang v1.8.0
	github.com/pierrec/lz4/v4 v4.1.17
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_golang v1.14.0
	github.com/prometheus/common v0.42.0 // indirect
	github.com/prometheus/procfs v0.9.0 // indirect
	github.com/quic-go/quic-go v0.33.0
	github.com/rcrowley/go-metrics v0.0.0-20201227073835-cf1acfcdf475
	github.com/sasha-s/go-deadlock v0.3.1
	github.com/shirou/gopsutil/v3 v3.23.2
	github.com/syncthing/notify v0.0.0-20210616190510-c6b7342338d2
	github.com/syndtr/goleveldb v1.0.1-0.20220721030215-126854af5e6d
	github.com/thejerf/suture/v4 v4.0.2
	github.com/urfave/cli v1.22.12
	github.com/vitrun/qart v0.0.0-20160531060029-bf64b92db6b0
	golang.org/x/crypto v0.7.0
	golang.org/x/mod v0.9.0 // indirect
	golang.org/x/net v0.8.0
	golang.org/x/sys v0.6.0
	golang.org/x/text v0.8.0
	golang.org/x/time v0.3.0
	golang.org/x/tools v0.7.0
	google.golang.org/protobuf v1.29.0
)

require (
	github.com/Azure/go-ntlmssp v0.0.0-20221128193559-754e69321358 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-task/slim-sprig v0.0.0-20210107165309-348f09dbbbc0 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/pprof v0.0.0-20230309165930-d61513b1440d // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/onsi/ginkgo/v2 v2.9.0 // indirect
	github.com/oschwald/maxminddb-golang v1.10.0 // indirect
	github.com/petermattis/goid v0.0.0-20230222173705-8ff7bb262a50 // indirect
	github.com/power-devops/perfstat v0.0.0-20221212215047-62379fc7944b // indirect
	github.com/prometheus/client_model v0.3.0 // indirect
	github.com/quic-go/qtls-go1-19 v0.2.1 // indirect
	github.com/quic-go/qtls-go1-20 v0.1.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.2 // indirect
	golang.org/x/exp v0.0.0-20230307190834-24139beb5833 // indirect
)

// https://github.com/gobwas/glob/pull/55
replace github.com/gobwas/glob v0.2.3 => github.com/calmh/glob v0.0.0-20220615080505-1d823af5017b
