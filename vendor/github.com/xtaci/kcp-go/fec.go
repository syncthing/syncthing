package kcp

import (
	"encoding/binary"
	"sync/atomic"

	"github.com/klauspost/reedsolomon"
)

const (
	fecHeaderSize      = 6
	fecHeaderSizePlus2 = fecHeaderSize + 2 // plus 2B data size
	typeData           = 0xf1
	typeFEC            = 0xf2
	fecExpire          = 30000 // 30s
)

type (
	// FEC defines forward error correction for packets
	FEC struct {
		rx           []fecPacket // ordered receive queue
		rxlimit      int         // queue size limit
		dataShards   int
		parityShards int
		shardSize    int
		next         uint32 // next seqid
		enc          reedsolomon.Encoder
		shards       [][]byte
		shards2      [][]byte // for calcECC
		shardsflag   []bool
		paws         uint32 // Protect Against Wrapped Sequence numbers
		lastCheck    uint32
	}

	fecPacket struct {
		seqid uint32
		flag  uint16
		data  []byte
		ts    uint32
	}
)

func newFEC(rxlimit, dataShards, parityShards int) *FEC {
	if dataShards <= 0 || parityShards <= 0 {
		return nil
	}
	if rxlimit < dataShards+parityShards {
		return nil
	}

	fec := new(FEC)
	fec.rxlimit = rxlimit
	fec.dataShards = dataShards
	fec.parityShards = parityShards
	fec.shardSize = dataShards + parityShards
	fec.paws = (0xffffffff/uint32(fec.shardSize) - 1) * uint32(fec.shardSize)
	enc, err := reedsolomon.New(dataShards, parityShards)
	if err != nil {
		return nil
	}
	fec.enc = enc
	fec.shards = make([][]byte, fec.shardSize)
	fec.shards2 = make([][]byte, fec.shardSize)
	fec.shardsflag = make([]bool, fec.shardSize)
	return fec
}

// decode a fec packet
func (fec *FEC) decode(data []byte) fecPacket {
	var pkt fecPacket
	pkt.seqid = binary.LittleEndian.Uint32(data)
	pkt.flag = binary.LittleEndian.Uint16(data[4:])
	pkt.ts = currentMs()
	// allocate memory & copy
	buf := xmitBuf.Get().([]byte)[:len(data)-6]
	copy(buf, data[6:])
	pkt.data = buf
	return pkt
}

func (fec *FEC) markData(data []byte) {
	binary.LittleEndian.PutUint32(data, fec.next)
	binary.LittleEndian.PutUint16(data[4:], typeData)
	fec.next++
}

func (fec *FEC) markFEC(data []byte) {
	binary.LittleEndian.PutUint32(data, fec.next)
	binary.LittleEndian.PutUint16(data[4:], typeFEC)
	fec.next++
	if fec.next >= fec.paws { // paws would only occurs in markFEC
		fec.next = 0
	}
}

// input a fec packet
func (fec *FEC) input(pkt fecPacket) (recovered [][]byte) {
	// expiration
	now := currentMs()
	if now-fec.lastCheck >= fecExpire {
		var rx []fecPacket
		for k := range fec.rx {
			if now-fec.rx[k].ts < fecExpire {
				rx = append(rx, fec.rx[k])
			} else {
				xmitBuf.Put(fec.rx[k].data)
			}
		}
		fec.rx = rx
		fec.lastCheck = now
	}

	// insertion
	n := len(fec.rx) - 1
	insertIdx := 0
	for i := n; i >= 0; i-- {
		if pkt.seqid == fec.rx[i].seqid { // de-duplicate
			xmitBuf.Put(pkt.data)
			return nil
		} else if pkt.seqid > fec.rx[i].seqid { // insertion
			insertIdx = i + 1
			break
		}
	}

	// insert into ordered rx queue
	if insertIdx == n+1 {
		fec.rx = append(fec.rx, pkt)
	} else {
		fec.rx = append(fec.rx, fecPacket{})
		copy(fec.rx[insertIdx+1:], fec.rx[insertIdx:])
		fec.rx[insertIdx] = pkt
	}

	// shard range for current packet
	shardBegin := pkt.seqid - pkt.seqid%uint32(fec.shardSize)
	shardEnd := shardBegin + uint32(fec.shardSize) - 1

	// max search range in ordered queue for current shard
	searchBegin := insertIdx - int(pkt.seqid%uint32(fec.shardSize))
	if searchBegin < 0 {
		searchBegin = 0
	}
	searchEnd := searchBegin + fec.shardSize - 1
	if searchEnd >= len(fec.rx) {
		searchEnd = len(fec.rx) - 1
	}

	if searchEnd > searchBegin && searchEnd-searchBegin+1 >= fec.dataShards {
		numshard := 0
		numDataShard := 0
		first := -1
		maxlen := 0
		shards := fec.shards
		shardsflag := fec.shardsflag
		for k := range fec.shards {
			shards[k] = nil
			shardsflag[k] = false
		}

		for i := searchBegin; i <= searchEnd; i++ {
			seqid := fec.rx[i].seqid
			if seqid > shardEnd {
				break
			} else if seqid >= shardBegin {
				shards[seqid%uint32(fec.shardSize)] = fec.rx[i].data
				shardsflag[seqid%uint32(fec.shardSize)] = true
				numshard++
				if fec.rx[i].flag == typeData {
					numDataShard++
				}
				if numshard == 1 {
					first = i
				}
				if len(fec.rx[i].data) > maxlen {
					maxlen = len(fec.rx[i].data)
				}
			}
		}

		if numDataShard == fec.dataShards { // no lost
			for i := first; i < first+numshard; i++ { // free
				xmitBuf.Put(fec.rx[i].data)
			}
			copy(fec.rx[first:], fec.rx[first+numshard:])
			for i := 0; i < numshard; i++ { // dereference
				fec.rx[len(fec.rx)-1-i] = fecPacket{}
			}
			fec.rx = fec.rx[:len(fec.rx)-numshard]
		} else if numshard >= fec.dataShards { // recoverable
			for k := range shards {
				if shards[k] != nil {
					dlen := len(shards[k])
					shards[k] = shards[k][:maxlen]
					xorBytes(shards[k][dlen:], shards[k][dlen:], shards[k][dlen:])
				}
			}
			if err := fec.enc.Reconstruct(shards); err == nil {
				for k := range shards[:fec.dataShards] {
					if !shardsflag[k] {
						recovered = append(recovered, shards[k])
					}
				}
			}

			for i := first; i < first+numshard; i++ { // free
				xmitBuf.Put(fec.rx[i].data)
			}
			copy(fec.rx[first:], fec.rx[first+numshard:])
			for i := 0; i < numshard; i++ { // dereference
				fec.rx[len(fec.rx)-1-i] = fecPacket{}
			}
			fec.rx = fec.rx[:len(fec.rx)-numshard]
		}
	}

	// keep rxlimit
	if len(fec.rx) > fec.rxlimit {
		if fec.rx[0].flag == typeData { // record unrecoverable data
			atomic.AddUint64(&DefaultSnmp.FECShortShards, 1)
		}
		xmitBuf.Put(fec.rx[0].data) // free
		fec.rx[0].data = nil
		fec.rx = fec.rx[1:]
	}
	return
}

func (fec *FEC) calcECC(data [][]byte, offset, maxlen int) (ecc [][]byte) {
	if len(data) != fec.shardSize {
		return nil
	}
	shards := fec.shards2
	for k := range shards {
		shards[k] = data[k][offset:maxlen]
	}

	if err := fec.enc.Encode(shards); err != nil {
		return nil
	}
	return data[fec.dataShards:]
}
