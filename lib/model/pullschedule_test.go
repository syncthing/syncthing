package model

import (
	"github.com/d4l3k/messagediff"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
	"reflect"
	"testing"
)

var (
	someBlocks = []protocol.BlockInfo{{Offset: 1}, {Offset: 2}, {Offset: 3}}
)

func Test_chunk(t *testing.T) {
	type args struct {
		blocks    []protocol.BlockInfo
		partCount int
	}
	tests := []struct {
		name string
		args args
		want [][]protocol.BlockInfo
	}{
		{"one", args{someBlocks, 1}, [][]protocol.BlockInfo{someBlocks}},
		{"two", args{someBlocks, 2}, [][]protocol.BlockInfo{someBlocks[:2], someBlocks[2:]}},
		{"three", args{someBlocks, 3}, [][]protocol.BlockInfo{someBlocks[:1], someBlocks[1:2], someBlocks[2:]}},
		{"four", args{someBlocks, 4}, [][]protocol.BlockInfo{someBlocks[:1], someBlocks[1:2], someBlocks[2:]}},
		// Never happens as myIdx would be -1, so we'd return in order.
		{"zero", args{someBlocks, 0}, [][]protocol.BlockInfo{someBlocks}},
		{"empty-one", args{nil, 1}, [][]protocol.BlockInfo{}},
		{"empty-zero", args{nil, 0}, [][]protocol.BlockInfo{nil}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := chunk(tt.args.blocks, tt.args.partCount); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("chunk() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_contains(t *testing.T) {
	type args struct {
		devices []protocol.DeviceID
		id      protocol.DeviceID
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"basic1", args{[]protocol.DeviceID{device1, device2}, device1}, true},
		{"basic2", args{[]protocol.DeviceID{device2, device1}, device1}, true},
		{"basic3", args{[]protocol.DeviceID{device2}, device1}, false},
		{"basic4", args{nil, device1}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contains(tt.args.devices, tt.args.id); got != tt.want {
				t.Errorf("contains() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_inOrderPullSchedule_Reorder(t *testing.T) {
	type args struct {
		in0    []protocol.DeviceID
		in1    []protocol.DeviceID
		blocks []protocol.BlockInfo
	}
	tests := []struct {
		name string
		args args
		want []protocol.BlockInfo
	}{
		{"basic", args{nil, nil, someBlocks}, someBlocks},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := inOrderPullSchedule{}
			if got := in.Reorder(tt.args.in0, tt.args.in1, tt.args.blocks); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Reorder() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_model_getCommonDevicesSharingTheFolder(t *testing.T) {
	type fields struct {
		folder        string
		folderDevices []protocol.DeviceID
		ccMessages    map[protocol.DeviceID]protocol.ClusterConfig
	}
	makeCcFolder := func(folder string, ids ...protocol.DeviceID) protocol.Folder {
		devices := make([]protocol.Device, 0, len(ids))
		for _, id := range ids {
			devices = append(devices, protocol.Device{ID: id})
		}
		return protocol.Folder{ID: folder, Devices: devices}
	}
	tests := []struct {
		name   string
		fields fields
		folder string
		want   map[protocol.DeviceID][]protocol.DeviceID
	}{
		{"basic", fields{"folder", []protocol.DeviceID{device1, device2, device3}, map[protocol.DeviceID]protocol.ClusterConfig{
			device1: {[]protocol.Folder{
				makeCcFolder("folder", device2, device4),
				makeCcFolder("other", device3),
			}},
			device2: {[]protocol.Folder{
				makeCcFolder("folder", device3, device5),
				makeCcFolder("other", device2),
			}},
		}},
			"folder", map[protocol.DeviceID][]protocol.DeviceID{
				device1: {device2},
				device2: {device3},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			folderDevices := make([]config.FolderDeviceConfiguration, 0, len(tt.fields.folderDevices))
			for _, dev := range tt.fields.folderDevices {
				folderDevices = append(folderDevices, config.FolderDeviceConfiguration{DeviceID: dev})
			}
			m := &model{
				pmut:       sync.NewRWMutex(),
				fmut:       sync.NewRWMutex(),
				ccMessages: tt.fields.ccMessages,
				folderCfgs: map[string]config.FolderConfiguration{
					tt.fields.folder: {
						Devices: folderDevices,
					},
				},
			}
			got := m.getCommonDevicesSharingTheFolder(tt.folder)

			if diff, equals := messagediff.PrettyDiff(got, tt.want); !equals {
				t.Errorf("getCommonDevicesSharingTheFolder() = %v\nwant %v\ndiff:\n%v", got, tt.want, diff)
			}
		})
	}
}

func Test_standardPullSchedule_sharesFolderBothWays(t *testing.T) {
	type args struct {
		one   protocol.DeviceID
		other protocol.DeviceID
	}
	tests := []struct {
		name          string
		commonDevices map[protocol.DeviceID][]protocol.DeviceID
		args          args
		want          bool
	}{
		{"basic", map[protocol.DeviceID][]protocol.DeviceID{
			device1: {device2},
			device2: {device1},
		}, args{device1, device2}, true},
		{"basic-order", map[protocol.DeviceID][]protocol.DeviceID{
			device1: {device2},
			device2: {device1},
		}, args{device2, device1}, true},
		{"missing", map[protocol.DeviceID][]protocol.DeviceID{
			device1: {},
			device2: {device1},
		}, args{device1, device2}, false},
		{"missing-other", map[protocol.DeviceID][]protocol.DeviceID{
			device1: {device2},
			device2: {},
		}, args{device1, device2}, false},
		{"missing-both", map[protocol.DeviceID][]protocol.DeviceID{
			device1: {},
			device2: {},
		}, args{device1, device2}, false},
		{"one-nil", map[protocol.DeviceID][]protocol.DeviceID{
			device1: {device2},
		}, args{device1, device2}, false},
		{"nil-both", map[protocol.DeviceID][]protocol.DeviceID{}, args{device1, device2}, false},
		{"missing-but-has-something", map[protocol.DeviceID][]protocol.DeviceID{
			device1: {device1},
			device2: {device2},
		}, args{device1, device2}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := standardPullSchedule{
				commonDevices: tt.commonDevices,
			}
			if got := p.sharesFolderBothWays(tt.args.one, tt.args.other); got != tt.want {
				t.Errorf("sharesFolderBothWays() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_standardPullSchedule_devicesThatNeedTheFileAndCanGetItDirectly(t *testing.T) {
	type fields struct {
		myId          protocol.DeviceID
		commonDevices map[protocol.DeviceID][]protocol.DeviceID
	}
	type args struct {
		connectedWithFile      []protocol.DeviceID
		connectedSharingFolder []protocol.DeviceID
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   []protocol.DeviceID
	}{
		{
			"one-source",
			fields{
				device1,
				map[protocol.DeviceID][]protocol.DeviceID{
					device2: {device3, device6},
					device3: {device2, device4},
					device4: {device3}, // Can only get it via 3
					device5: {device2}, // 2 does not share back,
					device6: {},        // 6 does not share back with 2
				},
			},
			args{
				[]protocol.DeviceID{device2},
				[]protocol.DeviceID{device2, device3, device4, device5, device6},
			},
			[]protocol.DeviceID{device1, device3},
		},
		{
			"two-sources",
			fields{
				device1,
				map[protocol.DeviceID][]protocol.DeviceID{
					device2: {device3},
					device3: {device2},
					device4: {device5, device6},
					device5: {device4},
					device6: {device4}, // "offline"
				},
			},
			args{
				[]protocol.DeviceID{device2, device4},
				[]protocol.DeviceID{device2, device3, device4, device5},
			},
			[]protocol.DeviceID{device1, device3, device5},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &standardPullSchedule{
				myId:          tt.fields.myId,
				commonDevices: tt.fields.commonDevices,
			}
			if got := p.devicesThatNeedTheFileAndCanGetItDirectly(tt.args.connectedWithFile, tt.args.connectedSharingFolder); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("devicesThatNeedTheFileAndCanGetItDirectly() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_standardPullSchedule_reorderBlocksForDevices(t *testing.T) {
	type args struct {
		devices []protocol.DeviceID
		blocks  []protocol.BlockInfo
	}
	blocks := func(i ...int) []protocol.BlockInfo {
		b := make([]protocol.BlockInfo, 0, len(i))
		for _, v := range i {
			b = append(b, protocol.BlockInfo{Offset: int64(v)})
		}
		return b
	}
	tests := []struct {
		name string
		myId protocol.DeviceID
		args args
		want []protocol.BlockInfo
	}{
		{"front", device1, args{[]protocol.DeviceID{device1, device2, device3}, blocks(1, 2, 3)}, blocks(1, 2, 3)},
		{"back", device1, args{[]protocol.DeviceID{device2, device3, device1}, blocks(1, 2, 3)}, blocks(3, 1, 2)},
		{"few-blocks", device1, args{[]protocol.DeviceID{device3, device2, device1}, blocks(1)}, blocks(1)},
		{"more-than-one-block", device1, args{[]protocol.DeviceID{device2, device1}, blocks(1, 2, 3, 4)}, blocks(3, 4, 1, 2)},
		{"empty-blocks", device1, args{[]protocol.DeviceID{device2, device1}, blocks()}, blocks()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &standardPullSchedule{
				myId:    tt.myId,
				shuffle: func(i interface{}) {}, // Noop shuffle
			}
			if got := p.reorderBlocksForDevices(tt.args.devices, tt.args.blocks); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("reorderBlocksForDevices() = %v, want %v", got, tt.want)
			}
		})
	}
}
