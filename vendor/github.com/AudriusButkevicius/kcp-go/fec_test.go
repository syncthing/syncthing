package kcp

import (
	"encoding/binary"
	"math/rand"
	"testing"
)

func TestFECOther(t *testing.T) {
	if newFEC(128, 0, 1) != nil {
		t.Fail()
	}
	if newFEC(128, 0, 0) != nil {
		t.Fail()
	}
	if newFEC(1, 10, 10) != nil {
		t.Fail()
	}
}

func TestFECNoLost(t *testing.T) {
	fec := newFEC(128, 10, 3)
	for i := 0; i < 100; i += 10 {
		data := makefecgroup(i, 13)
		for k := range data[fec.dataShards] {
			fec.markData(data[k])
			t.Log("input:", data[k])
		}

		ecc := fec.calcECC(data, fecHeaderSize, fecHeaderSize+4)
		for k := range ecc {
			fec.markFEC(ecc[k])
		}
		t.Log("  ecc:", ecc)
		data = append(data, ecc...)
		for k := range data {
			f := fec.decode(data[k])
			if recovered := fec.input(f); recovered != nil {
				for k := range recovered {
					t.Log("recovered:", binary.LittleEndian.Uint32(recovered[k]))
				}
			}
		}
	}
}

func TestFECLost1(t *testing.T) {
	fec := newFEC(128, 10, 3)
	println(fec.paws, fec.paws%13)
	fec.next = fec.paws - 13
	for i := 0; i < 100; i += 10 {
		data := makefecgroup(i, 13)
		for k := range data[fec.dataShards] {
			fec.markData(data[k])
			t.Log("input:", data[k])
		}
		ecc := fec.calcECC(data, fecHeaderSize, fecHeaderSize+4)
		for k := range ecc {
			fec.markFEC(ecc[k])
		}
		t.Log("ecc:", ecc)
		lost := rand.Intn(13)
		t.Log("lost:", data[lost])
		for k := range data {
			if k != lost {
				f := fec.decode(data[k])
				if recovered := fec.input(f); recovered != nil {
					for i := range recovered {
						t.Log("recovered:", binary.LittleEndian.Uint32(recovered[i]))
					}
				}
			}
		}
	}
}

func TestFECLost2(t *testing.T) {
	fec := newFEC(128, 10, 3)
	for i := 0; i < 100; i += 10 {
		data := makefecgroup(i, 13)
		for k := range data[fec.dataShards] {
			fec.markData(data[k])
			t.Log("input:", data[k])
		}
		ecc := fec.calcECC(data, fecHeaderSize, fecHeaderSize+4)
		for k := range ecc {
			fec.markFEC(ecc[k])
		}
		t.Log("ecc:", ecc)
		lost1, lost2 := rand.Intn(13), rand.Intn(13)
		t.Log(" lost1:", data[lost1])
		t.Log(" lost2:", data[lost2])
		for k := range data {
			if k != lost1 && k != lost2 {
				f := fec.decode(data[k])
				if recovered := fec.input(f); recovered != nil {
					for i := range recovered {
						t.Log("recovered:", binary.LittleEndian.Uint32(recovered[i]))
					}
				}
			}
		}
	}
}

func makefecgroup(start, size int) (group [][]byte) {
	for i := 0; i < size; i++ {
		data := make([]byte, fecHeaderSize+4)
		binary.LittleEndian.PutUint32(data[fecHeaderSize:], uint32(start+i))
		group = append(group, data)
	}
	return
}
