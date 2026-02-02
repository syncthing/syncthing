module github.com/syncthing/syncthing

go 1.24.0

require (
	github.com/AudriusButkevicius/recli v0.0.7
	github.com/Azure/azure-sdk-for-go/sdk/storage/azblob v1.6.2
	github.com/alecthomas/kong v1.12.1
	github.com/aws/aws-sdk-go v1.55.8
	github.com/calmh/incontainer v1.0.0
	github.com/calmh/xdr v1.2.0
	github.com/ccding/go-stun v0.1.5
	github.com/coreos/go-semver v0.3.1
	github.com/d4l3k/messagediff v1.2.1
	github.com/getsentry/raven-go v0.2.0
	github.com/go-ldap/ldap/v3 v3.4.11
	github.com/gobwas/glob v0.2.3
	github.com/gofrs/flock v0.12.1
	github.com/hashicorp/golang-lru/v2 v2.0.7
	github.com/jackpal/gateway v1.0.16
	github.com/jackpal/go-nat-pmp v1.0.2
	github.com/jmoiron/sqlx v1.4.0
	github.com/julienschmidt/httprouter v1.3.0
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/maruel/panicparse/v2 v2.5.0
	github.com/mattn/go-sqlite3 v1.14.31
	github.com/maxmind/geoipupdate/v6 v6.1.0
	github.com/miscreant/miscreant.go v0.0.0-20200214223636-26d376326b75
	github.com/oschwald/geoip2-golang v1.13.0
	github.com/pierrec/lz4/v4 v4.1.22
	github.com/prometheus/client_golang v1.23.0
	github.com/puzpuzpuz/xsync/v3 v3.5.1
	github.com/quic-go/quic-go v0.56.0
	github.com/rabbitmq/amqp091-go v1.10.0
	github.com/rcrowley/go-metrics v0.0.0-20250401214520-65e299d6c5c9
	github.com/shirou/gopsutil/v4 v4.25.6 // https://github.com/shirou/gopsutil/issues/1898
	github.com/syncthing/notify v0.0.0-20250528144937-c7027d4f7465
	github.com/syndtr/goleveldb v1.0.1-0.20220721030215-126854af5e6d
	github.com/thejerf/suture/v4 v4.0.6
	github.com/urfave/cli v1.22.17
	github.com/vitrun/qart v0.0.0-20160531060029-bf64b92db6b0
	github.com/willabides/kongplete v0.4.0
	github.com/wlynxg/anet v0.0.5
	go.uber.org/automaxprocs v1.6.0
	golang.org/x/crypto v0.44.0
	golang.org/x/exp v0.0.0-20250811191247-51f88131bc50
	golang.org/x/net v0.47.0
	golang.org/x/sys v0.38.0
	golang.org/x/text v0.31.0
	golang.org/x/time v0.12.0
	google.golang.org/protobuf v1.36.7
	modernc.org/sqlite v1.38.2
	sigs.k8s.io/yaml v1.6.0
)

require (
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.18.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.11.1 // indirect
	github.com/Azure/go-ntlmssp v0.0.0-20221128193559-754e69321358 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/certifi/gocertifi v0.0.0-20210507211836-431795d63e8d // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/ebitengine/purego v0.8.4 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-asn1-ber/asn1-ber v1.5.8-0.20250403174932-29230038a667 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/pprof v0.0.0-20250423184734-337e5dd93bb4 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20240909124753-873cd0166683 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/maxbrunsfeld/counterfeiter/v6 v6.12.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/ncruces/go-strftime v0.1.9 // indirect
	github.com/nxadm/tail v1.4.11 // indirect
	github.com/oschwald/maxminddb-golang v1.13.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/posener/complete v1.2.3 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.65.0 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/riywo/loginshell v0.0.0-20200815045211-7d26008be1ab // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/stretchr/testify v1.10.0 // indirect
	github.com/tklauser/go-sysconf v0.3.15 // indirect
	github.com/tklauser/numcpus v0.10.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/mod v0.30.0 // indirect
	golang.org/x/sync v0.18.0 // indirect
	golang.org/x/telemetry v0.0.0-20251111182119-bc8e575c7b54 // indirect
	golang.org/x/tools v0.39.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	modernc.org/libc v1.66.3 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

// https://github.com/gobwas/glob/pull/55
replace github.com/gobwas/glob v0.2.3 => github.com/calmh/glob v0.0.0-20220615080505-1d823af5017b

// https://github.com/mattn/go-sqlite3/pull/1338
replace github.com/mattn/go-sqlite3 v1.14.31 => github.com/calmh/go-sqlite3 v1.14.32-0.20250812195006-80712c77b76a

tool (
	github.com/calmh/xdr/cmd/genxdr
	github.com/maxbrunsfeld/counterfeiter/v6
	golang.org/x/tools/cmd/goimports
)
