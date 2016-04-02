package glob

import (
	"fmt"
	"reflect"
	"testing"
)

func TestParseString(t *testing.T) {
	for id, test := range []struct {
		pattern string
		tree    node
	}{
		{
			pattern: "abc",
			tree: &nodePattern{
				nodeImpl: nodeImpl{
					desc: []node{
						&nodeText{text: "abc"},
					},
				},
			},
		},
		{
			pattern: "a*c",
			tree: &nodePattern{
				nodeImpl: nodeImpl{
					desc: []node{
						&nodeText{text: "a"},
						&nodeAny{},
						&nodeText{text: "c"},
					},
				},
			},
		},
		{
			pattern: "a**c",
			tree: &nodePattern{
				nodeImpl: nodeImpl{
					desc: []node{
						&nodeText{text: "a"},
						&nodeSuper{},
						&nodeText{text: "c"},
					},
				},
			},
		},
		{
			pattern: "a?c",
			tree: &nodePattern{
				nodeImpl: nodeImpl{
					desc: []node{
						&nodeText{text: "a"},
						&nodeSingle{},
						&nodeText{text: "c"},
					},
				},
			},
		},
		{
			pattern: "[!a-z]",
			tree: &nodePattern{
				nodeImpl: nodeImpl{
					desc: []node{
						&nodeRange{lo: 'a', hi: 'z', not: true},
					},
				},
			},
		},
		{
			pattern: "[az]",
			tree: &nodePattern{
				nodeImpl: nodeImpl{
					desc: []node{
						&nodeList{chars: "az"},
					},
				},
			},
		},
		{
			pattern: "{a,z}",
			tree: &nodePattern{
				nodeImpl: nodeImpl{
					desc: []node{
						&nodeAnyOf{nodeImpl: nodeImpl{desc: []node{
							&nodePattern{
								nodeImpl: nodeImpl{desc: []node{
									&nodeText{text: "a"},
								}},
							},
							&nodePattern{
								nodeImpl: nodeImpl{desc: []node{
									&nodeText{text: "z"},
								}},
							},
						}}},
					},
				},
			},
		},
		{
			pattern: "{a,{x,y},?,[a-z],[!qwe]}",
			tree: &nodePattern{
				nodeImpl: nodeImpl{
					desc: []node{
						&nodeAnyOf{nodeImpl: nodeImpl{desc: []node{
							&nodePattern{
								nodeImpl: nodeImpl{desc: []node{
									&nodeText{text: "a"},
								}},
							},
							&nodePattern{
								nodeImpl: nodeImpl{desc: []node{
									&nodeAnyOf{nodeImpl: nodeImpl{desc: []node{
										&nodePattern{
											nodeImpl: nodeImpl{desc: []node{
												&nodeText{text: "x"},
											}},
										},
										&nodePattern{
											nodeImpl: nodeImpl{desc: []node{
												&nodeText{text: "y"},
											}},
										},
									}}},
								}},
							},
							&nodePattern{
								nodeImpl: nodeImpl{desc: []node{
									&nodeSingle{},
								}},
							},
							&nodePattern{
								nodeImpl: nodeImpl{
									desc: []node{
										&nodeRange{lo: 'a', hi: 'z', not: false},
									},
								},
							},
							&nodePattern{
								nodeImpl: nodeImpl{
									desc: []node{
										&nodeList{chars: "qwe", not: true},
									},
								},
							},
						}}},
					},
				},
			},
		},
	} {
		pattern, err := parse(newLexer(test.pattern))
		if err != nil {
			t.Errorf("#%d %s", id, err)
			continue
		}

		if !reflect.DeepEqual(test.tree, pattern) {
			t.Errorf("#%d tries are not equal", id)
			if err = nodeEqual(test.tree, pattern); err != nil {
				t.Errorf("#%d %s", id, err)
				continue
			}
		}
	}
}

const abstractNodeImpl = "nodeImpl"

func nodeEqual(a, b node) error {
	if (a == nil || b == nil) && a != b {
		return fmt.Errorf("nodes are not equal: exp %s, act %s", a, b)
	}

	aValue, bValue := reflect.Indirect(reflect.ValueOf(a)), reflect.Indirect(reflect.ValueOf(b))
	aType, bType := aValue.Type(), bValue.Type()
	if aType != bType {
		return fmt.Errorf("nodes are not equal: exp %s, act %s", aValue.Type(), bValue.Type())
	}

	for i := 0; i < aType.NumField(); i++ {
		var eq bool

		f := aType.Field(i).Name
		if f == abstractNodeImpl {
			continue
		}

		af, bf := aValue.FieldByName(f), bValue.FieldByName(f)

		switch af.Kind() {
		case reflect.String:
			eq = af.String() == bf.String()
		case reflect.Bool:
			eq = af.Bool() == bf.Bool()
		default:
			eq = fmt.Sprint(af) == fmt.Sprint(bf)
		}

		if !eq {
			return fmt.Errorf("nodes<%s> %q fields are not equal: exp %q, act %q", aType, f, af, bf)
		}
	}

	for i, aDesc := range a.children() {
		if len(b.children())-1 < i {
			return fmt.Errorf("node does not have enough children (got %d children, wanted %d-th token)", len(b.children()), i)
		}

		bDesc := b.children()[i]

		if err := nodeEqual(aDesc, bDesc); err != nil {
			return err
		}
	}

	return nil
}
