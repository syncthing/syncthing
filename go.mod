module github.com/syncthing/syncthing

go 1.26.2

require (
	buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go v1.36.12-20260825204119-511051f7f437.2
	buf.build/go/protovalidate v1.4.1
	github.com/AudriusButkevicius/recli v0.0.7
	github.com/alecthomas/kong v1.17.0
	github.com/aws/aws-sdk-go v1.55.8
	github.com/calmh/incontainer v1.0.0
	github.com/calmh/xdr v1.2.0
	github.com/ccding/go-stun v0.1.6
	github.com/coreos/go-semver v0.3.1
	github.com/d4l3k/messagediff v1.2.1
	github.com/getsentry/raven-go v0.2.0
	github.com/go-ldap/ldap/v3 v3.4.15
	github.com/gobwas/glob v1.1.0
	github.com/gofrs/flock v0.13.2
	github.com/hashicorp/golang-lru/v2 v2.0.7
	github.com/ipfs/go-datastore v0.9.2
	github.com/ipfs/go-log/v2 v2.9.2
	github.com/jackpal/gateway v1.2.0
	github.com/jackpal/go-nat-pmp v1.1.0
	github.com/jmoiron/sqlx v1.4.0
	github.com/julienschmidt/httprouter v1.3.0
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/libp2p/go-libp2p v0.52.0
	github.com/libp2p/go-libp2p-kad-dht v0.43.1
	github.com/libp2p/go-libp2p-record v0.3.1
	github.com/maruel/panicparse/v2 v2.5.0
	github.com/mattn/go-sqlite3 v1.14.53
	github.com/maxmind/geoipupdate/v6 v6.1.0
	github.com/miscreant/miscreant.go v0.0.0-20200214223636-26d376326b75
	github.com/multiformats/go-multiaddr v0.16.1
	github.com/oschwald/geoip2-golang v1.13.0
	github.com/pierrec/lz4/v4 v4.1.34
	github.com/prometheus/client_golang v1.25.0
	github.com/puzpuzpuz/xsync/v3 v3.5.1
	github.com/quic-go/quic-go v0.64.1
	github.com/rabbitmq/amqp091-go v1.17.0
	github.com/rcrowley/go-metrics v0.0.0-20250401214520-65e299d6c5c9
	github.com/shirou/gopsutil/v4 v4.26.10
	github.com/syncthing/notify v0.0.0-20250528144937-c7027d4f7465
	github.com/syndtr/goleveldb v1.0.1-0.20220721030215-126854af5e6d
	github.com/thejerf/suture/v4 v4.0.6
	github.com/urfave/cli v1.22.17
	github.com/vitrun/qart v0.0.0-20160531060029-bf64b92db6b0
	github.com/willabides/kongplete v0.4.0
	github.com/wlynxg/anet v0.0.5
	go.uber.org/zap v1.28.0
	golang.org/x/crypto v0.59.0
	golang.org/x/exp v0.0.0-20261124000051-d6646c509b6d
	golang.org/x/net v0.63.0
	golang.org/x/sys v0.50.0
	golang.org/x/text v0.44.0
	golang.org/x/time v0.16.0
	google.golang.org/protobuf v1.36.12
	modernc.org/sqlite v1.63.0
	sigs.k8s.io/yaml v1.6.0
)

require (
	cel.dev/cel-go v0.33.2 // indirect
	cel.dev/expr v0.25.3 // indirect
	filippo.io/bigmod v0.1.1-0.20260103110540-f8a47775ebe5 // indirect
	filippo.io/keygen v1.0.0 // indirect
	github.com/Azure/go-ntlmssp v0.1.1 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/benbjohnson/clock v1.3.5 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/certifi/gocertifi v0.0.0-20210507211836-431795d63e8d // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/davidlazar/go-crypto v0.0.0-20200604182044-b73af7476f6c // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.1 // indirect
	github.com/dunglas/httpsfv v1.1.2 // indirect
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/ebitengine/purego v0.11.2 // indirect
	github.com/filecoin-project/go-clock v0.1.0 // indirect
	github.com/flynn/noise v1.1.0 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-asn1-ber/asn1-ber v1.5.8 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/golang/snappy v0.0.5-0.20231225225746-43d5d4cd4e0e // indirect
	github.com/google/gopacket v1.1.19 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/huin/goupnp v1.3.1 // indirect
	github.com/ipfs/boxo v0.43.0 // indirect
	github.com/ipfs/go-cid v0.6.2 // indirect
	github.com/ipld/go-ipld-prime v0.24.0 // indirect
	github.com/jbenet/go-temp-err-catcher v0.1.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/klauspost/cpuid/v2 v2.4.2 // indirect
	github.com/koron/go-ssdp v0.9.2 // indirect
	github.com/libp2p/go-buffer-pool v0.1.0 // indirect
	github.com/libp2p/go-cidranger v1.1.0 // indirect
	github.com/libp2p/go-flow-metrics v0.3.0 // indirect
	github.com/libp2p/go-libp2p-asn-util v0.4.1 // indirect
	github.com/libp2p/go-libp2p-kbucket v0.9.0 // indirect
	github.com/libp2p/go-libp2p-routing-helpers v0.7.5 // indirect
	github.com/libp2p/go-msgio v0.3.0 // indirect
	github.com/libp2p/go-netroute v0.4.0 // indirect
	github.com/libp2p/go-reuseport v0.4.0 // indirect
	github.com/libp2p/go-yamux/v5 v5.1.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20240909124753-873cd0166683 // indirect
	github.com/marten-seemann/tcp v0.0.0-20210406111302-dfbc87cc63fd // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/maxbrunsfeld/counterfeiter/v6 v6.12.0 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/mikioh/tcpinfo v0.0.0-20190314235526-30a79bb1804b // indirect
	github.com/mikioh/tcpopt v0.0.0-20190314235656-172688c1accc // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/mr-tron/base58 v1.3.0 // indirect
	github.com/multiformats/go-base32 v0.1.0 // indirect
	github.com/multiformats/go-base36 v0.2.0 // indirect
	github.com/multiformats/go-multiaddr-dns v0.6.0 // indirect
	github.com/multiformats/go-multiaddr-fmt v0.1.0 // indirect
	github.com/multiformats/go-multibase v0.3.0 // indirect
	github.com/multiformats/go-multicodec v0.10.0 // indirect
	github.com/multiformats/go-multihash v0.3.0 // indirect
	github.com/multiformats/go-multistream v0.6.1 // indirect
	github.com/multiformats/go-varint v0.1.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/nxadm/tail v1.4.11 // indirect
	github.com/oschwald/maxminddb-golang v1.13.1 // indirect
	github.com/pandatix/go-abnf v0.5.5 // indirect
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58 // indirect
	github.com/pion/datachannel v1.6.4 // indirect
	github.com/pion/dtls/v3 v3.1.16 // indirect
	github.com/pion/ice/v4 v4.4.9 // indirect
	github.com/pion/interceptor v0.1.51 // indirect
	github.com/pion/logging v0.2.4 // indirect
	github.com/pion/mdns/v2 v2.2.2 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/rtcp v1.2.20 // indirect
	github.com/pion/rtp v1.10.6 // indirect
	github.com/pion/sctp v1.12.2 // indirect
	github.com/pion/sdp/v3 v3.0.20 // indirect
	github.com/pion/srtp/v3 v3.1.3 // indirect
	github.com/pion/stun/v3 v3.1.7 // indirect
	github.com/pion/stun/v4 v4.0.1 // indirect
	github.com/pion/transport/v4 v4.1.3 // indirect
	github.com/pion/transport/v5 v5.1.6 // indirect
	github.com/pion/turn/v5 v5.1.3 // indirect
	github.com/pion/webrtc/v4 v4.2.29 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/polydawn/refmt v0.90.0 // indirect
	github.com/posener/complete v1.2.3 // indirect
	github.com/power-devops/perfstat v0.0.0-20260805114148-88456608a4f6 // indirect
	github.com/prometheus/client_model v0.6.3 // indirect
	github.com/prometheus/common v0.74.0 // indirect
	github.com/prometheus/procfs v0.23.0 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/quic-go/webtransport-go v0.13.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/riywo/loginshell v0.0.0-20200815045211-7d26008be1ab // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/tklauser/go-sysconf v0.4.0 // indirect
	github.com/tklauser/numcpus v0.12.0 // indirect
	github.com/whyrusleeping/go-keyspace v0.0.0-20160322163242-5b898ac5add1 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/auto/sdk v1.2.2 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	go.uber.org/dig v1.19.0 // indirect
	go.uber.org/fx v1.24.0 // indirect
	go.uber.org/mock v0.6.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v2 v2.4.5 // indirect
	go.yaml.in/yaml/v3 v3.0.6 // indirect
	golang.org/x/mod v0.43.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/telemetry v0.0.0-20261124234621-1402bdb29cb5 // indirect
	golang.org/x/tools v0.53.0 // indirect
	gonum.org/v1/gonum v0.17.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260819154853-08b0e4226688 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260819154853-08b0e4226688 // indirect
	lukechampine.com/blake3 v1.5.3 // indirect
	modernc.org/libc v1.80.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)

// https://github.com/gobwas/glob/pull/55
replace github.com/gobwas/glob v0.2.3 => github.com/calmh/glob v0.0.0-20220615080505-1d823af5017b

// https://github.com/jackpal/gateway/pull/49
replace github.com/jackpal/gateway v1.1.1 => github.com/marbens-arch/gateway v1.1.2-0.20260308173556-c567cc04e7d4

tool (
	github.com/calmh/xdr/cmd/genxdr
	github.com/maxbrunsfeld/counterfeiter/v6
	golang.org/x/tools/cmd/goimports
)
