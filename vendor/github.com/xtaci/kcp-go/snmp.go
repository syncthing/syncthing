package kcp

import (
	"fmt"
	"sync/atomic"
)

// Snmp defines network statistics indicator
type Snmp struct {
	BytesSent        uint64 // raw bytes sent
	BytesReceived    uint64
	MaxConn          uint64
	ActiveOpens      uint64
	PassiveOpens     uint64
	CurrEstab        uint64 // count of connections for now
	InErrs           uint64 // udp read errors
	InCsumErrors     uint64 // checksum errors from CRC32
	KCPInErrors      uint64 // packet iput errors from kcp
	InSegs           uint64
	OutSegs          uint64
	InBytes          uint64 // udp bytes received
	OutBytes         uint64 // udp bytes sent
	RetransSegs      uint64
	FastRetransSegs  uint64
	EarlyRetransSegs uint64
	LostSegs         uint64 // number of segs infered as lost
	RepeatSegs       uint64 // number of segs duplicated
	FECRecovered     uint64 // correct packets recovered from FEC
	FECErrs          uint64 // incorrect packets recovered from FEC
	FECSegs          uint64 // FEC segments received
	FECShortShards   uint64 // number of data shards that's not enough for recovery
}

func newSnmp() *Snmp {
	return new(Snmp)
}

func (s *Snmp) Header() []string {
	return []string{
		"BytesSent",
		"BytesReceived",
		"MaxConn",
		"ActiveOpens",
		"PassiveOpens",
		"CurrEstab",
		"InErrs",
		"InCsumErrors",
		"KCPInErrors",
		"InSegs",
		"OutSegs",
		"InBytes",
		"OutBytes",
		"RetransSegs",
		"FastRetransSegs",
		"EarlyRetransSegs",
		"LostSegs",
		"RepeatSegs",
		"FECSegs",
		"FECErrs",
		"FECRecovered",
		"FECShortShards",
	}
}

func (s *Snmp) ToSlice() []string {
	snmp := s.Copy()
	return []string{
		fmt.Sprint(snmp.BytesSent),
		fmt.Sprint(snmp.BytesReceived),
		fmt.Sprint(snmp.MaxConn),
		fmt.Sprint(snmp.ActiveOpens),
		fmt.Sprint(snmp.PassiveOpens),
		fmt.Sprint(snmp.CurrEstab),
		fmt.Sprint(snmp.InErrs),
		fmt.Sprint(snmp.InCsumErrors),
		fmt.Sprint(snmp.KCPInErrors),
		fmt.Sprint(snmp.InSegs),
		fmt.Sprint(snmp.OutSegs),
		fmt.Sprint(snmp.InBytes),
		fmt.Sprint(snmp.OutBytes),
		fmt.Sprint(snmp.RetransSegs),
		fmt.Sprint(snmp.FastRetransSegs),
		fmt.Sprint(snmp.EarlyRetransSegs),
		fmt.Sprint(snmp.LostSegs),
		fmt.Sprint(snmp.RepeatSegs),
		fmt.Sprint(snmp.FECSegs),
		fmt.Sprint(snmp.FECErrs),
		fmt.Sprint(snmp.FECRecovered),
		fmt.Sprint(snmp.FECShortShards),
	}
}

// Copy make a copy of current snmp snapshot
func (s *Snmp) Copy() *Snmp {
	d := newSnmp()
	d.BytesSent = atomic.LoadUint64(&s.BytesSent)
	d.BytesReceived = atomic.LoadUint64(&s.BytesReceived)
	d.MaxConn = atomic.LoadUint64(&s.MaxConn)
	d.ActiveOpens = atomic.LoadUint64(&s.ActiveOpens)
	d.PassiveOpens = atomic.LoadUint64(&s.PassiveOpens)
	d.CurrEstab = atomic.LoadUint64(&s.CurrEstab)
	d.InErrs = atomic.LoadUint64(&s.InErrs)
	d.InCsumErrors = atomic.LoadUint64(&s.InCsumErrors)
	d.KCPInErrors = atomic.LoadUint64(&s.KCPInErrors)
	d.InSegs = atomic.LoadUint64(&s.InSegs)
	d.OutSegs = atomic.LoadUint64(&s.OutSegs)
	d.InBytes = atomic.LoadUint64(&s.InBytes)
	d.OutBytes = atomic.LoadUint64(&s.OutBytes)
	d.RetransSegs = atomic.LoadUint64(&s.RetransSegs)
	d.FastRetransSegs = atomic.LoadUint64(&s.FastRetransSegs)
	d.EarlyRetransSegs = atomic.LoadUint64(&s.EarlyRetransSegs)
	d.LostSegs = atomic.LoadUint64(&s.LostSegs)
	d.RepeatSegs = atomic.LoadUint64(&s.RepeatSegs)
	d.FECSegs = atomic.LoadUint64(&s.FECSegs)
	d.FECErrs = atomic.LoadUint64(&s.FECErrs)
	d.FECRecovered = atomic.LoadUint64(&s.FECRecovered)
	d.FECShortShards = atomic.LoadUint64(&s.FECShortShards)
	return d
}

// Reset values to zero
func (s *Snmp) Reset() {
	atomic.StoreUint64(&s.BytesSent, 0)
	atomic.StoreUint64(&s.BytesReceived, 0)
	atomic.StoreUint64(&s.MaxConn, 0)
	atomic.StoreUint64(&s.ActiveOpens, 0)
	atomic.StoreUint64(&s.PassiveOpens, 0)
	atomic.StoreUint64(&s.CurrEstab, 0)
	atomic.StoreUint64(&s.InErrs, 0)
	atomic.StoreUint64(&s.InCsumErrors, 0)
	atomic.StoreUint64(&s.KCPInErrors, 0)
	atomic.StoreUint64(&s.InSegs, 0)
	atomic.StoreUint64(&s.OutSegs, 0)
	atomic.StoreUint64(&s.InBytes, 0)
	atomic.StoreUint64(&s.OutBytes, 0)
	atomic.StoreUint64(&s.RetransSegs, 0)
	atomic.StoreUint64(&s.FastRetransSegs, 0)
	atomic.StoreUint64(&s.EarlyRetransSegs, 0)
	atomic.StoreUint64(&s.LostSegs, 0)
	atomic.StoreUint64(&s.RepeatSegs, 0)
	atomic.StoreUint64(&s.FECSegs, 0)
	atomic.StoreUint64(&s.FECErrs, 0)
	atomic.StoreUint64(&s.FECRecovered, 0)
	atomic.StoreUint64(&s.FECShortShards, 0)
}

// DefaultSnmp is the global KCP connection statistics collector
var DefaultSnmp *Snmp

func init() {
	DefaultSnmp = newSnmp()
}
