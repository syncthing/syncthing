package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strconv"
	"strings"
	"text/template"
)

var output string

type field struct {
	Name      string
	IsBasic   bool
	IsSlice   bool
	IsMap     bool
	FieldType string
	KeyType   string
	Encoder   string
	Convert   string
	Max       int
}

var headerTpl = template.Must(template.New("header").Parse(`package {{.Package}}

import (
	"bytes"
	"io"

	"github.com/calmh/syncthing/xdr"
)
`))

var encodeTpl = template.Must(template.New("encoder").Parse(`
func (o {{.TypeName}}) EncodeXDR(w io.Writer) (int, error) {
	var xw = xdr.NewWriter(w)
	return o.encodeXDR(xw)
}//+n

func (o {{.TypeName}}) MarshalXDR() []byte {
	var buf bytes.Buffer
	var xw = xdr.NewWriter(&buf)
	o.encodeXDR(xw)
	return buf.Bytes()
}//+n

func (o {{.TypeName}}) encodeXDR(xw *xdr.Writer) (int, error) {
	{{range $field := .Fields}}
	{{if not $field.IsSlice}}
		{{if ne $field.Convert ""}}
		xw.Write{{$field.Encoder}}({{$field.Convert}}(o.{{$field.Name}}))
		{{else if $field.IsBasic}}
		{{if ge $field.Max 1}}
		if len(o.{{$field.Name}}) > {{$field.Max}} {
			return xw.Tot(), xdr.ErrElementSizeExceeded
		}
		{{end}}
		xw.Write{{$field.Encoder}}(o.{{$field.Name}})
		{{else}}
		o.{{$field.Name}}.encodeXDR(xw)
		{{end}}
	{{else}}
	{{if ge $field.Max 1}}
	if len(o.{{$field.Name}}) > {{$field.Max}} {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	{{end}}
	xw.WriteUint32(uint32(len(o.{{$field.Name}})))
	for i := range o.{{$field.Name}} {
		{{if ne $field.Convert ""}}
		xw.Write{{$field.Encoder}}({{$field.Convert}}(o.{{$field.Name}}[i]))
		{{else if $field.IsBasic}}
		xw.Write{{$field.Encoder}}(o.{{$field.Name}}[i])
		{{else}}
		o.{{$field.Name}}[i].encodeXDR(xw)
		{{end}}
	}
	{{end}}
	{{end}}
	return xw.Tot(), xw.Error()
}//+n

func (o *{{.TypeName}}) DecodeXDR(r io.Reader) error {
	xr := xdr.NewReader(r)
	return o.decodeXDR(xr)
}//+n

func (o *{{.TypeName}}) UnmarshalXDR(bs []byte) error {
	var buf = bytes.NewBuffer(bs)
	var xr = xdr.NewReader(buf)
	return o.decodeXDR(xr)
}//+n

func (o *{{.TypeName}}) decodeXDR(xr *xdr.Reader) error {
	{{range $field := .Fields}}
	{{if not $field.IsSlice}}
		{{if ne $field.Convert ""}}
		o.{{$field.Name}} = {{$field.FieldType}}(xr.Read{{$field.Encoder}}())
		{{else if $field.IsBasic}}
		{{if ge $field.Max 1}}
		o.{{$field.Name}} = xr.Read{{$field.Encoder}}Max({{$field.Max}})
		{{else}}
		o.{{$field.Name}} = xr.Read{{$field.Encoder}}()
		{{end}}
		{{else}}
		(&o.{{$field.Name}}).decodeXDR(xr)
		{{end}}
	{{else}}
	_{{$field.Name}}Size := int(xr.ReadUint32())
	{{if ge $field.Max 1}}
	if _{{$field.Name}}Size > {{$field.Max}} {
		return xdr.ErrElementSizeExceeded
	}
	{{end}}
	o.{{$field.Name}} = make([]{{$field.FieldType}}, _{{$field.Name}}Size)
	for i := range o.{{$field.Name}} {
		{{if ne $field.Convert ""}}
		o.{{$field.Name}}[i] = {{$field.FieldType}}(xr.Read{{$field.Encoder}}())
		{{else if $field.IsBasic}}
		o.{{$field.Name}}[i] = xr.Read{{$field.Encoder}}()
		{{else}}
		(&o.{{$field.Name}}[i]).decodeXDR(xr)
		{{end}}
	}
	{{end}}
	{{end}}
	return xr.Error()
}`))

var maxRe = regexp.MustCompile(`\Wmax:(\d+)`)

type typeSet struct {
	Type    string
	Encoder string
}

var xdrEncoders = map[string]typeSet{
	"int16":  typeSet{"uint16", "Uint16"},
	"uint16": typeSet{"", "Uint16"},
	"int32":  typeSet{"uint32", "Uint32"},
	"uint32": typeSet{"", "Uint32"},
	"int64":  typeSet{"uint64", "Uint64"},
	"uint64": typeSet{"", "Uint64"},
	"int":    typeSet{"uint64", "Uint64"},
	"string": typeSet{"", "String"},
	"[]byte": typeSet{"", "Bytes"},
	"bool":   typeSet{"", "Bool"},
}

func handleStruct(name string, t *ast.StructType) {
	var fs []field
	for _, sf := range t.Fields.List {
		if len(sf.Names) == 0 {
			// We don't handle anonymous fields
			continue
		}

		fn := sf.Names[0].Name
		var max = 0
		if sf.Comment != nil {
			c := sf.Comment.List[0].Text
			if m := maxRe.FindStringSubmatch(c); m != nil {
				max, _ = strconv.Atoi(m[1])
			}
		}

		var f field
		switch ft := sf.Type.(type) {
		case *ast.Ident:
			tn := ft.Name
			if enc, ok := xdrEncoders[tn]; ok {
				f = field{
					Name:      fn,
					IsBasic:   true,
					FieldType: tn,
					Encoder:   enc.Encoder,
					Convert:   enc.Type,
					Max:       max,
				}
			} else {
				f = field{
					Name:      fn,
					IsBasic:   false,
					FieldType: tn,
					Max:       max,
				}
			}

		case *ast.ArrayType:
			if ft.Len != nil {
				// We don't handle arrays
				continue
			}

			tn := ft.Elt.(*ast.Ident).Name
			if enc, ok := xdrEncoders["[]"+tn]; ok {
				f = field{
					Name:      fn,
					IsBasic:   true,
					FieldType: tn,
					Encoder:   enc.Encoder,
					Convert:   enc.Type,
					Max:       max,
				}
			} else if enc, ok := xdrEncoders[tn]; ok {
				f = field{
					Name:      fn,
					IsBasic:   true,
					IsSlice:   true,
					FieldType: tn,
					Encoder:   enc.Encoder,
					Convert:   enc.Type,
					Max:       max,
				}
			} else {
				f = field{
					Name:      fn,
					IsBasic:   false,
					IsSlice:   true,
					FieldType: tn,
					Max:       max,
				}
			}
		}

		fs = append(fs, f)
	}

	switch output {
	case "code":
		generateCode(name, fs)
	case "diagram":
		generateDiagram(name, fs)
	case "xdr":
		generateXdr(name, fs)
	}
}

func generateCode(name string, fs []field) {
	var buf bytes.Buffer
	err := encodeTpl.Execute(&buf, map[string]interface{}{"TypeName": name, "Fields": fs})
	if err != nil {
		panic(err)
	}

	bs := regexp.MustCompile(`(\s*\n)+`).ReplaceAll(buf.Bytes(), []byte("\n"))
	bs = bytes.Replace(bs, []byte("//+n"), []byte("\n"), -1)

	bs, err = format.Source(bs)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(bs))
}

func generateDiagram(sn string, fs []field) {
	fmt.Println(sn + " Structure:")
	fmt.Println()
	fmt.Println(" 0                   1                   2                   3")
	fmt.Println(" 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1")
	line := "+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+"
	fmt.Println(line)

	for _, f := range fs {
		tn := f.FieldType
		sl := f.IsSlice

		if sl {
			fmt.Printf("| %s |\n", center("Number of "+f.Name, 61))
			fmt.Println(line)
		}
		switch tn {
		case "uint16":
			fmt.Printf("| %s | %s |\n", center(f.Name, 29), center("0x0000", 29))
			fmt.Println(line)
		case "uint32":
			fmt.Printf("| %s |\n", center(f.Name, 61))
			fmt.Println(line)
		case "int64", "uint64":
			fmt.Printf("| %-61s |\n", "")
			fmt.Printf("+ %s +\n", center(f.Name+" (64 bits)", 61))
			fmt.Printf("| %-61s |\n", "")
			fmt.Println(line)
		case "string", "byte": // XXX We assume slice of byte!
			fmt.Printf("| %s |\n", center("Length of "+f.Name, 61))
			fmt.Println(line)
			fmt.Printf("/ %61s /\n", "")
			fmt.Printf("\\ %s \\\n", center(f.Name+" (variable length)", 61))
			fmt.Printf("/ %61s /\n", "")
			fmt.Println(line)
		default:
			if sl {
				tn = "Zero or more " + tn + " Structures"
				fmt.Printf("/ %s /\n", center("", 61))
				fmt.Printf("\\ %s \\\n", center(tn, 61))
				fmt.Printf("/ %s /\n", center("", 61))
			} else {
				fmt.Printf("| %s |\n", center(tn, 61))
			}
			fmt.Println(line)
		}
	}
	fmt.Println()
	fmt.Println()
}

func generateXdr(sn string, fs []field) {
	fmt.Printf("struct %s {\n", sn)

	for _, f := range fs {
		tn := f.FieldType
		fn := f.Name
		suf := ""
		if f.IsSlice {
			suf = "<>"
		}

		switch tn {
		case "uint16":
			fmt.Printf("\tunsigned short %s%s;\n", fn, suf)
		case "uint32":
			fmt.Printf("\tunsigned int %s%s;\n", fn, suf)
		case "int64":
			fmt.Printf("\thyper %s%s;\n", fn, suf)
		case "uint64":
			fmt.Printf("\tunsigned hyper %s%s;\n", fn, suf)
		case "string":
			fmt.Printf("\tstring %s<>;\n", fn)
		case "byte":
			fmt.Printf("\topaque %s<>;\n", fn)
		default:
			fmt.Printf("\t%s %s%s;\n", tn, fn, suf)
		}
	}
	fmt.Println("}")
	fmt.Println()
}

func center(s string, w int) string {
	w -= len(s)
	l := w / 2
	r := l
	if l+r < w {
		r++
	}
	return strings.Repeat(" ", l) + s + strings.Repeat(" ", r)
}

func inspector(fset *token.FileSet) func(ast.Node) bool {
	return func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.TypeSpec:
			switch t := n.Type.(type) {
			case *ast.StructType:
				name := n.Name.Name
				handleStruct(name, t)
			}
			return false
		default:
			return true
		}
	}
}

func main() {
	flag.StringVar(&output, "output", "code", "code,xdr,diagram")
	flag.Parse()
	fname := flag.Arg(0)

	// Create the AST by parsing src.
	fset := token.NewFileSet() // positions are relative to fset
	f, err := parser.ParseFile(fset, fname, nil, parser.ParseComments)
	if err != nil {
		panic(err)
	}

	//ast.Print(fset, f)

	if output == "code" {
		headerTpl.Execute(os.Stdout, map[string]string{"Package": f.Name.Name})
	}

	i := inspector(fset)
	ast.Inspect(f, i)
}
