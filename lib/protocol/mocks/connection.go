// Code generated by counterfeiter. DO NOT EDIT.
package mocks

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

type Connection struct {
	CloseStub        func(error)
	closeMutex       sync.RWMutex
	closeArgsForCall []struct {
		arg1 error
	}
	ClosedStub        func() bool
	closedMutex       sync.RWMutex
	closedArgsForCall []struct {
	}
	closedReturns struct {
		result1 bool
	}
	closedReturnsOnCall map[int]struct {
		result1 bool
	}
	ClusterConfigStub        func(protocol.ClusterConfig)
	clusterConfigMutex       sync.RWMutex
	clusterConfigArgsForCall []struct {
		arg1 protocol.ClusterConfig
	}
	CryptoStub        func() string
	cryptoMutex       sync.RWMutex
	cryptoArgsForCall []struct {
	}
	cryptoReturns struct {
		result1 string
	}
	cryptoReturnsOnCall map[int]struct {
		result1 string
	}
	DownloadProgressStub        func(context.Context, string, []protocol.FileDownloadProgressUpdate)
	downloadProgressMutex       sync.RWMutex
	downloadProgressArgsForCall []struct {
		arg1 context.Context
		arg2 string
		arg3 []protocol.FileDownloadProgressUpdate
	}
	EstablishedAtStub        func() time.Time
	establishedAtMutex       sync.RWMutex
	establishedAtArgsForCall []struct {
	}
	establishedAtReturns struct {
		result1 time.Time
	}
	establishedAtReturnsOnCall map[int]struct {
		result1 time.Time
	}
	IDStub        func() protocol.DeviceID
	iDMutex       sync.RWMutex
	iDArgsForCall []struct {
	}
	iDReturns struct {
		result1 protocol.DeviceID
	}
	iDReturnsOnCall map[int]struct {
		result1 protocol.DeviceID
	}
	IndexStub        func(context.Context, string, []protocol.FileInfo) error
	indexMutex       sync.RWMutex
	indexArgsForCall []struct {
		arg1 context.Context
		arg2 string
		arg3 []protocol.FileInfo
	}
	indexReturns struct {
		result1 error
	}
	indexReturnsOnCall map[int]struct {
		result1 error
	}
	IndexUpdateStub        func(context.Context, string, []protocol.FileInfo) error
	indexUpdateMutex       sync.RWMutex
	indexUpdateArgsForCall []struct {
		arg1 context.Context
		arg2 string
		arg3 []protocol.FileInfo
	}
	indexUpdateReturns struct {
		result1 error
	}
	indexUpdateReturnsOnCall map[int]struct {
		result1 error
	}
	PriorityStub        func() int
	priorityMutex       sync.RWMutex
	priorityArgsForCall []struct {
	}
	priorityReturns struct {
		result1 int
	}
	priorityReturnsOnCall map[int]struct {
		result1 int
	}
	RemoteAddrStub        func() net.Addr
	remoteAddrMutex       sync.RWMutex
	remoteAddrArgsForCall []struct {
	}
	remoteAddrReturns struct {
		result1 net.Addr
	}
	remoteAddrReturnsOnCall map[int]struct {
		result1 net.Addr
	}
	RequestStub        func(context.Context, string, string, int, int64, int, []byte, uint32, bool) ([]byte, error)
	requestMutex       sync.RWMutex
	requestArgsForCall []struct {
		arg1 context.Context
		arg2 string
		arg3 string
		arg4 int
		arg5 int64
		arg6 int
		arg7 []byte
		arg8 uint32
		arg9 bool
	}
	requestReturns struct {
		result1 []byte
		result2 error
	}
	requestReturnsOnCall map[int]struct {
		result1 []byte
		result2 error
	}
	StartStub        func()
	startMutex       sync.RWMutex
	startArgsForCall []struct {
	}
	StatisticsStub        func() protocol.Statistics
	statisticsMutex       sync.RWMutex
	statisticsArgsForCall []struct {
	}
	statisticsReturns struct {
		result1 protocol.Statistics
	}
	statisticsReturnsOnCall map[int]struct {
		result1 protocol.Statistics
	}
	StringStub        func() string
	stringMutex       sync.RWMutex
	stringArgsForCall []struct {
	}
	stringReturns struct {
		result1 string
	}
	stringReturnsOnCall map[int]struct {
		result1 string
	}
	TransportStub        func() string
	transportMutex       sync.RWMutex
	transportArgsForCall []struct {
	}
	transportReturns struct {
		result1 string
	}
	transportReturnsOnCall map[int]struct {
		result1 string
	}
	TypeStub        func() string
	typeMutex       sync.RWMutex
	typeArgsForCall []struct {
	}
	typeReturns struct {
		result1 string
	}
	typeReturnsOnCall map[int]struct {
		result1 string
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *Connection) Close(arg1 error) {
	fake.closeMutex.Lock()
	fake.closeArgsForCall = append(fake.closeArgsForCall, struct {
		arg1 error
	}{arg1})
	stub := fake.CloseStub
	fake.recordInvocation("Close", []interface{}{arg1})
	fake.closeMutex.Unlock()
	if stub != nil {
		fake.CloseStub(arg1)
	}
}

func (fake *Connection) CloseCallCount() int {
	fake.closeMutex.RLock()
	defer fake.closeMutex.RUnlock()
	return len(fake.closeArgsForCall)
}

func (fake *Connection) CloseCalls(stub func(error)) {
	fake.closeMutex.Lock()
	defer fake.closeMutex.Unlock()
	fake.CloseStub = stub
}

func (fake *Connection) CloseArgsForCall(i int) error {
	fake.closeMutex.RLock()
	defer fake.closeMutex.RUnlock()
	argsForCall := fake.closeArgsForCall[i]
	return argsForCall.arg1
}

func (fake *Connection) Closed() bool {
	fake.closedMutex.Lock()
	ret, specificReturn := fake.closedReturnsOnCall[len(fake.closedArgsForCall)]
	fake.closedArgsForCall = append(fake.closedArgsForCall, struct {
	}{})
	stub := fake.ClosedStub
	fakeReturns := fake.closedReturns
	fake.recordInvocation("Closed", []interface{}{})
	fake.closedMutex.Unlock()
	if stub != nil {
		return stub()
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *Connection) ClosedCallCount() int {
	fake.closedMutex.RLock()
	defer fake.closedMutex.RUnlock()
	return len(fake.closedArgsForCall)
}

func (fake *Connection) ClosedCalls(stub func() bool) {
	fake.closedMutex.Lock()
	defer fake.closedMutex.Unlock()
	fake.ClosedStub = stub
}

func (fake *Connection) ClosedReturns(result1 bool) {
	fake.closedMutex.Lock()
	defer fake.closedMutex.Unlock()
	fake.ClosedStub = nil
	fake.closedReturns = struct {
		result1 bool
	}{result1}
}

func (fake *Connection) ClosedReturnsOnCall(i int, result1 bool) {
	fake.closedMutex.Lock()
	defer fake.closedMutex.Unlock()
	fake.ClosedStub = nil
	if fake.closedReturnsOnCall == nil {
		fake.closedReturnsOnCall = make(map[int]struct {
			result1 bool
		})
	}
	fake.closedReturnsOnCall[i] = struct {
		result1 bool
	}{result1}
}

func (fake *Connection) ClusterConfig(arg1 protocol.ClusterConfig) {
	fake.clusterConfigMutex.Lock()
	fake.clusterConfigArgsForCall = append(fake.clusterConfigArgsForCall, struct {
		arg1 protocol.ClusterConfig
	}{arg1})
	stub := fake.ClusterConfigStub
	fake.recordInvocation("ClusterConfig", []interface{}{arg1})
	fake.clusterConfigMutex.Unlock()
	if stub != nil {
		fake.ClusterConfigStub(arg1)
	}
}

func (fake *Connection) ClusterConfigCallCount() int {
	fake.clusterConfigMutex.RLock()
	defer fake.clusterConfigMutex.RUnlock()
	return len(fake.clusterConfigArgsForCall)
}

func (fake *Connection) ClusterConfigCalls(stub func(protocol.ClusterConfig)) {
	fake.clusterConfigMutex.Lock()
	defer fake.clusterConfigMutex.Unlock()
	fake.ClusterConfigStub = stub
}

func (fake *Connection) ClusterConfigArgsForCall(i int) protocol.ClusterConfig {
	fake.clusterConfigMutex.RLock()
	defer fake.clusterConfigMutex.RUnlock()
	argsForCall := fake.clusterConfigArgsForCall[i]
	return argsForCall.arg1
}

func (fake *Connection) Crypto() string {
	fake.cryptoMutex.Lock()
	ret, specificReturn := fake.cryptoReturnsOnCall[len(fake.cryptoArgsForCall)]
	fake.cryptoArgsForCall = append(fake.cryptoArgsForCall, struct {
	}{})
	stub := fake.CryptoStub
	fakeReturns := fake.cryptoReturns
	fake.recordInvocation("Crypto", []interface{}{})
	fake.cryptoMutex.Unlock()
	if stub != nil {
		return stub()
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *Connection) CryptoCallCount() int {
	fake.cryptoMutex.RLock()
	defer fake.cryptoMutex.RUnlock()
	return len(fake.cryptoArgsForCall)
}

func (fake *Connection) CryptoCalls(stub func() string) {
	fake.cryptoMutex.Lock()
	defer fake.cryptoMutex.Unlock()
	fake.CryptoStub = stub
}

func (fake *Connection) CryptoReturns(result1 string) {
	fake.cryptoMutex.Lock()
	defer fake.cryptoMutex.Unlock()
	fake.CryptoStub = nil
	fake.cryptoReturns = struct {
		result1 string
	}{result1}
}

func (fake *Connection) CryptoReturnsOnCall(i int, result1 string) {
	fake.cryptoMutex.Lock()
	defer fake.cryptoMutex.Unlock()
	fake.CryptoStub = nil
	if fake.cryptoReturnsOnCall == nil {
		fake.cryptoReturnsOnCall = make(map[int]struct {
			result1 string
		})
	}
	fake.cryptoReturnsOnCall[i] = struct {
		result1 string
	}{result1}
}

func (fake *Connection) DownloadProgress(arg1 context.Context, arg2 string, arg3 []protocol.FileDownloadProgressUpdate) {
	var arg3Copy []protocol.FileDownloadProgressUpdate
	if arg3 != nil {
		arg3Copy = make([]protocol.FileDownloadProgressUpdate, len(arg3))
		copy(arg3Copy, arg3)
	}
	fake.downloadProgressMutex.Lock()
	fake.downloadProgressArgsForCall = append(fake.downloadProgressArgsForCall, struct {
		arg1 context.Context
		arg2 string
		arg3 []protocol.FileDownloadProgressUpdate
	}{arg1, arg2, arg3Copy})
	stub := fake.DownloadProgressStub
	fake.recordInvocation("DownloadProgress", []interface{}{arg1, arg2, arg3Copy})
	fake.downloadProgressMutex.Unlock()
	if stub != nil {
		fake.DownloadProgressStub(arg1, arg2, arg3)
	}
}

func (fake *Connection) DownloadProgressCallCount() int {
	fake.downloadProgressMutex.RLock()
	defer fake.downloadProgressMutex.RUnlock()
	return len(fake.downloadProgressArgsForCall)
}

func (fake *Connection) DownloadProgressCalls(stub func(context.Context, string, []protocol.FileDownloadProgressUpdate)) {
	fake.downloadProgressMutex.Lock()
	defer fake.downloadProgressMutex.Unlock()
	fake.DownloadProgressStub = stub
}

func (fake *Connection) DownloadProgressArgsForCall(i int) (context.Context, string, []protocol.FileDownloadProgressUpdate) {
	fake.downloadProgressMutex.RLock()
	defer fake.downloadProgressMutex.RUnlock()
	argsForCall := fake.downloadProgressArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *Connection) EstablishedAt() time.Time {
	fake.establishedAtMutex.Lock()
	ret, specificReturn := fake.establishedAtReturnsOnCall[len(fake.establishedAtArgsForCall)]
	fake.establishedAtArgsForCall = append(fake.establishedAtArgsForCall, struct {
	}{})
	stub := fake.EstablishedAtStub
	fakeReturns := fake.establishedAtReturns
	fake.recordInvocation("EstablishedAt", []interface{}{})
	fake.establishedAtMutex.Unlock()
	if stub != nil {
		return stub()
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *Connection) EstablishedAtCallCount() int {
	fake.establishedAtMutex.RLock()
	defer fake.establishedAtMutex.RUnlock()
	return len(fake.establishedAtArgsForCall)
}

func (fake *Connection) EstablishedAtCalls(stub func() time.Time) {
	fake.establishedAtMutex.Lock()
	defer fake.establishedAtMutex.Unlock()
	fake.EstablishedAtStub = stub
}

func (fake *Connection) EstablishedAtReturns(result1 time.Time) {
	fake.establishedAtMutex.Lock()
	defer fake.establishedAtMutex.Unlock()
	fake.EstablishedAtStub = nil
	fake.establishedAtReturns = struct {
		result1 time.Time
	}{result1}
}

func (fake *Connection) EstablishedAtReturnsOnCall(i int, result1 time.Time) {
	fake.establishedAtMutex.Lock()
	defer fake.establishedAtMutex.Unlock()
	fake.EstablishedAtStub = nil
	if fake.establishedAtReturnsOnCall == nil {
		fake.establishedAtReturnsOnCall = make(map[int]struct {
			result1 time.Time
		})
	}
	fake.establishedAtReturnsOnCall[i] = struct {
		result1 time.Time
	}{result1}
}

func (fake *Connection) ID() protocol.DeviceID {
	fake.iDMutex.Lock()
	ret, specificReturn := fake.iDReturnsOnCall[len(fake.iDArgsForCall)]
	fake.iDArgsForCall = append(fake.iDArgsForCall, struct {
	}{})
	stub := fake.IDStub
	fakeReturns := fake.iDReturns
	fake.recordInvocation("ID", []interface{}{})
	fake.iDMutex.Unlock()
	if stub != nil {
		return stub()
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *Connection) IDCallCount() int {
	fake.iDMutex.RLock()
	defer fake.iDMutex.RUnlock()
	return len(fake.iDArgsForCall)
}

func (fake *Connection) IDCalls(stub func() protocol.DeviceID) {
	fake.iDMutex.Lock()
	defer fake.iDMutex.Unlock()
	fake.IDStub = stub
}

func (fake *Connection) IDReturns(result1 protocol.DeviceID) {
	fake.iDMutex.Lock()
	defer fake.iDMutex.Unlock()
	fake.IDStub = nil
	fake.iDReturns = struct {
		result1 protocol.DeviceID
	}{result1}
}

func (fake *Connection) IDReturnsOnCall(i int, result1 protocol.DeviceID) {
	fake.iDMutex.Lock()
	defer fake.iDMutex.Unlock()
	fake.IDStub = nil
	if fake.iDReturnsOnCall == nil {
		fake.iDReturnsOnCall = make(map[int]struct {
			result1 protocol.DeviceID
		})
	}
	fake.iDReturnsOnCall[i] = struct {
		result1 protocol.DeviceID
	}{result1}
}

func (fake *Connection) Index(arg1 context.Context, arg2 string, arg3 []protocol.FileInfo) error {
	var arg3Copy []protocol.FileInfo
	if arg3 != nil {
		arg3Copy = make([]protocol.FileInfo, len(arg3))
		copy(arg3Copy, arg3)
	}
	fake.indexMutex.Lock()
	ret, specificReturn := fake.indexReturnsOnCall[len(fake.indexArgsForCall)]
	fake.indexArgsForCall = append(fake.indexArgsForCall, struct {
		arg1 context.Context
		arg2 string
		arg3 []protocol.FileInfo
	}{arg1, arg2, arg3Copy})
	stub := fake.IndexStub
	fakeReturns := fake.indexReturns
	fake.recordInvocation("Index", []interface{}{arg1, arg2, arg3Copy})
	fake.indexMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *Connection) IndexCallCount() int {
	fake.indexMutex.RLock()
	defer fake.indexMutex.RUnlock()
	return len(fake.indexArgsForCall)
}

func (fake *Connection) IndexCalls(stub func(context.Context, string, []protocol.FileInfo) error) {
	fake.indexMutex.Lock()
	defer fake.indexMutex.Unlock()
	fake.IndexStub = stub
}

func (fake *Connection) IndexArgsForCall(i int) (context.Context, string, []protocol.FileInfo) {
	fake.indexMutex.RLock()
	defer fake.indexMutex.RUnlock()
	argsForCall := fake.indexArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *Connection) IndexReturns(result1 error) {
	fake.indexMutex.Lock()
	defer fake.indexMutex.Unlock()
	fake.IndexStub = nil
	fake.indexReturns = struct {
		result1 error
	}{result1}
}

func (fake *Connection) IndexReturnsOnCall(i int, result1 error) {
	fake.indexMutex.Lock()
	defer fake.indexMutex.Unlock()
	fake.IndexStub = nil
	if fake.indexReturnsOnCall == nil {
		fake.indexReturnsOnCall = make(map[int]struct {
			result1 error
		})
	}
	fake.indexReturnsOnCall[i] = struct {
		result1 error
	}{result1}
}

func (fake *Connection) IndexUpdate(arg1 context.Context, arg2 string, arg3 []protocol.FileInfo) error {
	var arg3Copy []protocol.FileInfo
	if arg3 != nil {
		arg3Copy = make([]protocol.FileInfo, len(arg3))
		copy(arg3Copy, arg3)
	}
	fake.indexUpdateMutex.Lock()
	ret, specificReturn := fake.indexUpdateReturnsOnCall[len(fake.indexUpdateArgsForCall)]
	fake.indexUpdateArgsForCall = append(fake.indexUpdateArgsForCall, struct {
		arg1 context.Context
		arg2 string
		arg3 []protocol.FileInfo
	}{arg1, arg2, arg3Copy})
	stub := fake.IndexUpdateStub
	fakeReturns := fake.indexUpdateReturns
	fake.recordInvocation("IndexUpdate", []interface{}{arg1, arg2, arg3Copy})
	fake.indexUpdateMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *Connection) IndexUpdateCallCount() int {
	fake.indexUpdateMutex.RLock()
	defer fake.indexUpdateMutex.RUnlock()
	return len(fake.indexUpdateArgsForCall)
}

func (fake *Connection) IndexUpdateCalls(stub func(context.Context, string, []protocol.FileInfo) error) {
	fake.indexUpdateMutex.Lock()
	defer fake.indexUpdateMutex.Unlock()
	fake.IndexUpdateStub = stub
}

func (fake *Connection) IndexUpdateArgsForCall(i int) (context.Context, string, []protocol.FileInfo) {
	fake.indexUpdateMutex.RLock()
	defer fake.indexUpdateMutex.RUnlock()
	argsForCall := fake.indexUpdateArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *Connection) IndexUpdateReturns(result1 error) {
	fake.indexUpdateMutex.Lock()
	defer fake.indexUpdateMutex.Unlock()
	fake.IndexUpdateStub = nil
	fake.indexUpdateReturns = struct {
		result1 error
	}{result1}
}

func (fake *Connection) IndexUpdateReturnsOnCall(i int, result1 error) {
	fake.indexUpdateMutex.Lock()
	defer fake.indexUpdateMutex.Unlock()
	fake.IndexUpdateStub = nil
	if fake.indexUpdateReturnsOnCall == nil {
		fake.indexUpdateReturnsOnCall = make(map[int]struct {
			result1 error
		})
	}
	fake.indexUpdateReturnsOnCall[i] = struct {
		result1 error
	}{result1}
}

func (fake *Connection) Priority() int {
	fake.priorityMutex.Lock()
	ret, specificReturn := fake.priorityReturnsOnCall[len(fake.priorityArgsForCall)]
	fake.priorityArgsForCall = append(fake.priorityArgsForCall, struct {
	}{})
	stub := fake.PriorityStub
	fakeReturns := fake.priorityReturns
	fake.recordInvocation("Priority", []interface{}{})
	fake.priorityMutex.Unlock()
	if stub != nil {
		return stub()
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *Connection) PriorityCallCount() int {
	fake.priorityMutex.RLock()
	defer fake.priorityMutex.RUnlock()
	return len(fake.priorityArgsForCall)
}

func (fake *Connection) PriorityCalls(stub func() int) {
	fake.priorityMutex.Lock()
	defer fake.priorityMutex.Unlock()
	fake.PriorityStub = stub
}

func (fake *Connection) PriorityReturns(result1 int) {
	fake.priorityMutex.Lock()
	defer fake.priorityMutex.Unlock()
	fake.PriorityStub = nil
	fake.priorityReturns = struct {
		result1 int
	}{result1}
}

func (fake *Connection) PriorityReturnsOnCall(i int, result1 int) {
	fake.priorityMutex.Lock()
	defer fake.priorityMutex.Unlock()
	fake.PriorityStub = nil
	if fake.priorityReturnsOnCall == nil {
		fake.priorityReturnsOnCall = make(map[int]struct {
			result1 int
		})
	}
	fake.priorityReturnsOnCall[i] = struct {
		result1 int
	}{result1}
}

func (fake *Connection) RemoteAddr() net.Addr {
	fake.remoteAddrMutex.Lock()
	ret, specificReturn := fake.remoteAddrReturnsOnCall[len(fake.remoteAddrArgsForCall)]
	fake.remoteAddrArgsForCall = append(fake.remoteAddrArgsForCall, struct {
	}{})
	stub := fake.RemoteAddrStub
	fakeReturns := fake.remoteAddrReturns
	fake.recordInvocation("RemoteAddr", []interface{}{})
	fake.remoteAddrMutex.Unlock()
	if stub != nil {
		return stub()
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *Connection) RemoteAddrCallCount() int {
	fake.remoteAddrMutex.RLock()
	defer fake.remoteAddrMutex.RUnlock()
	return len(fake.remoteAddrArgsForCall)
}

func (fake *Connection) RemoteAddrCalls(stub func() net.Addr) {
	fake.remoteAddrMutex.Lock()
	defer fake.remoteAddrMutex.Unlock()
	fake.RemoteAddrStub = stub
}

func (fake *Connection) RemoteAddrReturns(result1 net.Addr) {
	fake.remoteAddrMutex.Lock()
	defer fake.remoteAddrMutex.Unlock()
	fake.RemoteAddrStub = nil
	fake.remoteAddrReturns = struct {
		result1 net.Addr
	}{result1}
}

func (fake *Connection) RemoteAddrReturnsOnCall(i int, result1 net.Addr) {
	fake.remoteAddrMutex.Lock()
	defer fake.remoteAddrMutex.Unlock()
	fake.RemoteAddrStub = nil
	if fake.remoteAddrReturnsOnCall == nil {
		fake.remoteAddrReturnsOnCall = make(map[int]struct {
			result1 net.Addr
		})
	}
	fake.remoteAddrReturnsOnCall[i] = struct {
		result1 net.Addr
	}{result1}
}

func (fake *Connection) Request(arg1 context.Context, arg2 string, arg3 string, arg4 int, arg5 int64, arg6 int, arg7 []byte, arg8 uint32, arg9 bool) ([]byte, error) {
	var arg7Copy []byte
	if arg7 != nil {
		arg7Copy = make([]byte, len(arg7))
		copy(arg7Copy, arg7)
	}
	fake.requestMutex.Lock()
	ret, specificReturn := fake.requestReturnsOnCall[len(fake.requestArgsForCall)]
	fake.requestArgsForCall = append(fake.requestArgsForCall, struct {
		arg1 context.Context
		arg2 string
		arg3 string
		arg4 int
		arg5 int64
		arg6 int
		arg7 []byte
		arg8 uint32
		arg9 bool
	}{arg1, arg2, arg3, arg4, arg5, arg6, arg7Copy, arg8, arg9})
	stub := fake.RequestStub
	fakeReturns := fake.requestReturns
	fake.recordInvocation("Request", []interface{}{arg1, arg2, arg3, arg4, arg5, arg6, arg7Copy, arg8, arg9})
	fake.requestMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3, arg4, arg5, arg6, arg7, arg8, arg9)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *Connection) RequestCallCount() int {
	fake.requestMutex.RLock()
	defer fake.requestMutex.RUnlock()
	return len(fake.requestArgsForCall)
}

func (fake *Connection) RequestCalls(stub func(context.Context, string, string, int, int64, int, []byte, uint32, bool) ([]byte, error)) {
	fake.requestMutex.Lock()
	defer fake.requestMutex.Unlock()
	fake.RequestStub = stub
}

func (fake *Connection) RequestArgsForCall(i int) (context.Context, string, string, int, int64, int, []byte, uint32, bool) {
	fake.requestMutex.RLock()
	defer fake.requestMutex.RUnlock()
	argsForCall := fake.requestArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3, argsForCall.arg4, argsForCall.arg5, argsForCall.arg6, argsForCall.arg7, argsForCall.arg8, argsForCall.arg9
}

func (fake *Connection) RequestReturns(result1 []byte, result2 error) {
	fake.requestMutex.Lock()
	defer fake.requestMutex.Unlock()
	fake.RequestStub = nil
	fake.requestReturns = struct {
		result1 []byte
		result2 error
	}{result1, result2}
}

func (fake *Connection) RequestReturnsOnCall(i int, result1 []byte, result2 error) {
	fake.requestMutex.Lock()
	defer fake.requestMutex.Unlock()
	fake.RequestStub = nil
	if fake.requestReturnsOnCall == nil {
		fake.requestReturnsOnCall = make(map[int]struct {
			result1 []byte
			result2 error
		})
	}
	fake.requestReturnsOnCall[i] = struct {
		result1 []byte
		result2 error
	}{result1, result2}
}

func (fake *Connection) Start() {
	fake.startMutex.Lock()
	fake.startArgsForCall = append(fake.startArgsForCall, struct {
	}{})
	stub := fake.StartStub
	fake.recordInvocation("Start", []interface{}{})
	fake.startMutex.Unlock()
	if stub != nil {
		fake.StartStub()
	}
}

func (fake *Connection) StartCallCount() int {
	fake.startMutex.RLock()
	defer fake.startMutex.RUnlock()
	return len(fake.startArgsForCall)
}

func (fake *Connection) StartCalls(stub func()) {
	fake.startMutex.Lock()
	defer fake.startMutex.Unlock()
	fake.StartStub = stub
}

func (fake *Connection) Statistics() protocol.Statistics {
	fake.statisticsMutex.Lock()
	ret, specificReturn := fake.statisticsReturnsOnCall[len(fake.statisticsArgsForCall)]
	fake.statisticsArgsForCall = append(fake.statisticsArgsForCall, struct {
	}{})
	stub := fake.StatisticsStub
	fakeReturns := fake.statisticsReturns
	fake.recordInvocation("Statistics", []interface{}{})
	fake.statisticsMutex.Unlock()
	if stub != nil {
		return stub()
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *Connection) StatisticsCallCount() int {
	fake.statisticsMutex.RLock()
	defer fake.statisticsMutex.RUnlock()
	return len(fake.statisticsArgsForCall)
}

func (fake *Connection) StatisticsCalls(stub func() protocol.Statistics) {
	fake.statisticsMutex.Lock()
	defer fake.statisticsMutex.Unlock()
	fake.StatisticsStub = stub
}

func (fake *Connection) StatisticsReturns(result1 protocol.Statistics) {
	fake.statisticsMutex.Lock()
	defer fake.statisticsMutex.Unlock()
	fake.StatisticsStub = nil
	fake.statisticsReturns = struct {
		result1 protocol.Statistics
	}{result1}
}

func (fake *Connection) StatisticsReturnsOnCall(i int, result1 protocol.Statistics) {
	fake.statisticsMutex.Lock()
	defer fake.statisticsMutex.Unlock()
	fake.StatisticsStub = nil
	if fake.statisticsReturnsOnCall == nil {
		fake.statisticsReturnsOnCall = make(map[int]struct {
			result1 protocol.Statistics
		})
	}
	fake.statisticsReturnsOnCall[i] = struct {
		result1 protocol.Statistics
	}{result1}
}

func (fake *Connection) String() string {
	fake.stringMutex.Lock()
	ret, specificReturn := fake.stringReturnsOnCall[len(fake.stringArgsForCall)]
	fake.stringArgsForCall = append(fake.stringArgsForCall, struct {
	}{})
	stub := fake.StringStub
	fakeReturns := fake.stringReturns
	fake.recordInvocation("String", []interface{}{})
	fake.stringMutex.Unlock()
	if stub != nil {
		return stub()
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *Connection) StringCallCount() int {
	fake.stringMutex.RLock()
	defer fake.stringMutex.RUnlock()
	return len(fake.stringArgsForCall)
}

func (fake *Connection) StringCalls(stub func() string) {
	fake.stringMutex.Lock()
	defer fake.stringMutex.Unlock()
	fake.StringStub = stub
}

func (fake *Connection) StringReturns(result1 string) {
	fake.stringMutex.Lock()
	defer fake.stringMutex.Unlock()
	fake.StringStub = nil
	fake.stringReturns = struct {
		result1 string
	}{result1}
}

func (fake *Connection) StringReturnsOnCall(i int, result1 string) {
	fake.stringMutex.Lock()
	defer fake.stringMutex.Unlock()
	fake.StringStub = nil
	if fake.stringReturnsOnCall == nil {
		fake.stringReturnsOnCall = make(map[int]struct {
			result1 string
		})
	}
	fake.stringReturnsOnCall[i] = struct {
		result1 string
	}{result1}
}

func (fake *Connection) Transport() string {
	fake.transportMutex.Lock()
	ret, specificReturn := fake.transportReturnsOnCall[len(fake.transportArgsForCall)]
	fake.transportArgsForCall = append(fake.transportArgsForCall, struct {
	}{})
	stub := fake.TransportStub
	fakeReturns := fake.transportReturns
	fake.recordInvocation("Transport", []interface{}{})
	fake.transportMutex.Unlock()
	if stub != nil {
		return stub()
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *Connection) TransportCallCount() int {
	fake.transportMutex.RLock()
	defer fake.transportMutex.RUnlock()
	return len(fake.transportArgsForCall)
}

func (fake *Connection) TransportCalls(stub func() string) {
	fake.transportMutex.Lock()
	defer fake.transportMutex.Unlock()
	fake.TransportStub = stub
}

func (fake *Connection) TransportReturns(result1 string) {
	fake.transportMutex.Lock()
	defer fake.transportMutex.Unlock()
	fake.TransportStub = nil
	fake.transportReturns = struct {
		result1 string
	}{result1}
}

func (fake *Connection) TransportReturnsOnCall(i int, result1 string) {
	fake.transportMutex.Lock()
	defer fake.transportMutex.Unlock()
	fake.TransportStub = nil
	if fake.transportReturnsOnCall == nil {
		fake.transportReturnsOnCall = make(map[int]struct {
			result1 string
		})
	}
	fake.transportReturnsOnCall[i] = struct {
		result1 string
	}{result1}
}

func (fake *Connection) Type() string {
	fake.typeMutex.Lock()
	ret, specificReturn := fake.typeReturnsOnCall[len(fake.typeArgsForCall)]
	fake.typeArgsForCall = append(fake.typeArgsForCall, struct {
	}{})
	stub := fake.TypeStub
	fakeReturns := fake.typeReturns
	fake.recordInvocation("Type", []interface{}{})
	fake.typeMutex.Unlock()
	if stub != nil {
		return stub()
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *Connection) TypeCallCount() int {
	fake.typeMutex.RLock()
	defer fake.typeMutex.RUnlock()
	return len(fake.typeArgsForCall)
}

func (fake *Connection) TypeCalls(stub func() string) {
	fake.typeMutex.Lock()
	defer fake.typeMutex.Unlock()
	fake.TypeStub = stub
}

func (fake *Connection) TypeReturns(result1 string) {
	fake.typeMutex.Lock()
	defer fake.typeMutex.Unlock()
	fake.TypeStub = nil
	fake.typeReturns = struct {
		result1 string
	}{result1}
}

func (fake *Connection) TypeReturnsOnCall(i int, result1 string) {
	fake.typeMutex.Lock()
	defer fake.typeMutex.Unlock()
	fake.TypeStub = nil
	if fake.typeReturnsOnCall == nil {
		fake.typeReturnsOnCall = make(map[int]struct {
			result1 string
		})
	}
	fake.typeReturnsOnCall[i] = struct {
		result1 string
	}{result1}
}

func (fake *Connection) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.closeMutex.RLock()
	defer fake.closeMutex.RUnlock()
	fake.closedMutex.RLock()
	defer fake.closedMutex.RUnlock()
	fake.clusterConfigMutex.RLock()
	defer fake.clusterConfigMutex.RUnlock()
	fake.cryptoMutex.RLock()
	defer fake.cryptoMutex.RUnlock()
	fake.downloadProgressMutex.RLock()
	defer fake.downloadProgressMutex.RUnlock()
	fake.establishedAtMutex.RLock()
	defer fake.establishedAtMutex.RUnlock()
	fake.iDMutex.RLock()
	defer fake.iDMutex.RUnlock()
	fake.indexMutex.RLock()
	defer fake.indexMutex.RUnlock()
	fake.indexUpdateMutex.RLock()
	defer fake.indexUpdateMutex.RUnlock()
	fake.priorityMutex.RLock()
	defer fake.priorityMutex.RUnlock()
	fake.remoteAddrMutex.RLock()
	defer fake.remoteAddrMutex.RUnlock()
	fake.requestMutex.RLock()
	defer fake.requestMutex.RUnlock()
	fake.startMutex.RLock()
	defer fake.startMutex.RUnlock()
	fake.statisticsMutex.RLock()
	defer fake.statisticsMutex.RUnlock()
	fake.stringMutex.RLock()
	defer fake.stringMutex.RUnlock()
	fake.transportMutex.RLock()
	defer fake.transportMutex.RUnlock()
	fake.typeMutex.RLock()
	defer fake.typeMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *Connection) recordInvocation(key string, args []interface{}) {
	fake.invocationsMutex.Lock()
	defer fake.invocationsMutex.Unlock()
	if fake.invocations == nil {
		fake.invocations = map[string][][]interface{}{}
	}
	if fake.invocations[key] == nil {
		fake.invocations[key] = [][]interface{}{}
	}
	fake.invocations[key] = append(fake.invocations[key], args)
}

var _ protocol.Connection = new(Connection)
