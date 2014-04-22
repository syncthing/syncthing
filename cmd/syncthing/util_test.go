package main

import (
	"testing"

	"github.com/calmh/syncthing/protocol"
)

var testcases = []struct {
	local, remote protocol.ClusterConfigMessage
	err           string
}{
	{
		local:  protocol.ClusterConfigMessage{},
		remote: protocol.ClusterConfigMessage{},
		err:    "",
	},
	{
		local:  protocol.ClusterConfigMessage{ClientName: "a", ClientVersion: "b"},
		remote: protocol.ClusterConfigMessage{ClientName: "c", ClientVersion: "d"},
		err:    "",
	},
	{
		local: protocol.ClusterConfigMessage{
			Repositories: []protocol.Repository{
				{ID: "foo"},
			},
		},
		remote: protocol.ClusterConfigMessage{ClientName: "c", ClientVersion: "d"},
		err:    `remote is missing repository "foo"`,
	},
	{
		local: protocol.ClusterConfigMessage{ClientName: "c", ClientVersion: "d"},
		remote: protocol.ClusterConfigMessage{
			Repositories: []protocol.Repository{
				{ID: "foo"},
			},
		},
		err: `remote has extra repository "foo"`,
	},
	{
		local: protocol.ClusterConfigMessage{
			Repositories: []protocol.Repository{
				{ID: "foo"},
				{ID: "bar"},
			},
		},
		remote: protocol.ClusterConfigMessage{
			Repositories: []protocol.Repository{
				{ID: "foo"},
				{ID: "bar"},
			},
		},
		err: "",
	},
	{
		local: protocol.ClusterConfigMessage{
			Repositories: []protocol.Repository{
				{ID: "quux"},
				{ID: "foo"},
				{ID: "bar"},
			},
		},
		remote: protocol.ClusterConfigMessage{
			Repositories: []protocol.Repository{
				{ID: "bar"},
				{ID: "quux"},
			},
		},
		err: `remote is missing repository "foo"`,
	},
	{
		local: protocol.ClusterConfigMessage{
			Repositories: []protocol.Repository{
				{ID: "quux"},
				{ID: "bar"},
			},
		},
		remote: protocol.ClusterConfigMessage{
			Repositories: []protocol.Repository{
				{ID: "bar"},
				{ID: "foo"},
				{ID: "quux"},
			},
		},
		err: `remote has extra repository "foo"`,
	},
	{
		local: protocol.ClusterConfigMessage{
			Repositories: []protocol.Repository{
				{
					ID: "foo",
					Nodes: []protocol.Node{
						{ID: "a"},
					},
				},
				{ID: "bar"},
			},
		},
		remote: protocol.ClusterConfigMessage{
			Repositories: []protocol.Repository{
				{ID: "foo"},
				{ID: "bar"},
			},
		},
		err: "",
	},

	{
		local: protocol.ClusterConfigMessage{
			Repositories: []protocol.Repository{
				{
					ID: "foo",
					Nodes: []protocol.Node{
						{ID: "a"},
					},
				},
				{ID: "bar"},
			},
		},
		remote: protocol.ClusterConfigMessage{
			Repositories: []protocol.Repository{
				{
					ID: "foo",
					Nodes: []protocol.Node{
						{ID: "a"},
						{ID: "b"},
					},
				},
				{ID: "bar"},
			},
		},
		err: "",
	},

	{
		local: protocol.ClusterConfigMessage{
			Repositories: []protocol.Repository{
				{
					ID: "foo",
					Nodes: []protocol.Node{
						{
							ID:    "a",
							Flags: protocol.FlagShareReadOnly,
						},
					},
				},
				{ID: "bar"},
			},
		},
		remote: protocol.ClusterConfigMessage{
			Repositories: []protocol.Repository{
				{
					ID: "foo",
					Nodes: []protocol.Node{
						{
							ID:    "a",
							Flags: protocol.FlagShareTrusted,
						},
					},
				},
				{ID: "bar"},
			},
		},
		err: `remote has different sharing flags for node "a" in repository "foo"`,
	},
}

func TestCompareClusterConfig(t *testing.T) {
	for i, tc := range testcases {
		err := compareClusterConfig(tc.local, tc.remote)
		switch {
		case tc.err == "" && err != nil:
			t.Errorf("#%d: unexpected error: %v", i, err)

		case tc.err != "" && err == nil:
			t.Errorf("#%d: unexpected nil error", i)

		case tc.err != "" && err != nil && tc.err != err.Error():
			t.Errorf("#%d: incorrect error: %q != %q", i, err, tc.err)
		}
	}
}
