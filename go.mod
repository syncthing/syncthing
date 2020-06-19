module github.com/syncthing/syncthing

require (
	github.com/AudriusButkevicius/pfilter v0.0.0-20190627213056-c55ef6137fc6
	github.com/AudriusButkevicius/recli v0.0.5
	github.com/StackExchange/wmi v0.0.0-20190523213315-cbe66965904d // indirect
	github.com/bkaradzic/go-lz4 v0.0.0-20160924222819-7224d8d8f27e
	github.com/calmh/xdr v1.1.0
	github.com/ccding/go-stun v0.0.0-20180726100737-be486d185f3d
	github.com/certifi/gocertifi v0.0.0-20190905060710-a5e0173ced67 // indirect
	github.com/chmduquesne/rollinghash v0.0.0-20180912150627-a60f8e7142b5
	github.com/d4l3k/messagediff v1.2.1
	github.com/dgraph-io/badger/v2 v2.0.3
	github.com/flynn-archive/go-shlex v0.0.0-20150515145356-3f9db97f8568
	github.com/getsentry/raven-go v0.2.0
	github.com/go-ldap/ldap/v3 v3.1.10
	github.com/go-ole/go-ole v1.2.4 // indirect
	github.com/gobwas/glob v0.2.3
	github.com/gogo/protobuf v1.3.1
	github.com/golang/groupcache v0.0.0-20190702054246-869f871628b6
	github.com/greatroar/blobloom v0.2.1
	github.com/jackpal/gateway v1.0.6
	github.com/jackpal/go-nat-pmp v1.0.2
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/kr/pretty v0.2.0 // indirect
	github.com/lib/pq v1.2.0
	github.com/lucas-clemente/quic-go v0.17.1
	github.com/maruel/panicparse v1.3.0
	github.com/mattn/go-isatty v0.0.11
	github.com/minio/sha256-simd v0.1.1
	github.com/oschwald/geoip2-golang v1.4.0
	github.com/petermattis/goid v0.0.0-20180202154549-b0b1615b78e5 // indirect
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.2.1
	github.com/rcrowley/go-metrics v0.0.0-20190826022208-cac0b30c2563
	github.com/sasha-s/go-deadlock v0.2.0
	github.com/shirou/gopsutil v0.0.0-20190714054239-47ef3260b6bf
	github.com/syncthing/notify v0.0.0-20190709140112-69c7a957d3e2
	github.com/syndtr/goleveldb v1.0.1-0.20190923125748-758128399b1d
	github.com/thejerf/suture v3.0.2+incompatible
	github.com/urfave/cli v1.22.2
	github.com/vitrun/qart v0.0.0-20160531060029-bf64b92db6b0
	golang.org/x/crypto v0.0.0-20200423211502-4bdfaf469ed5
	golang.org/x/net v0.0.0-20190827160401-ba9fcec4b297
	golang.org/x/sys v0.0.0-20200223170610-d5e6a3e2c0ae
	golang.org/x/text v0.3.2
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4
)

go 1.13
