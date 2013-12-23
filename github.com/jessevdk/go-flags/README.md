go-flags: a go library for parsing command line arguments
=========================================================

This library provides similar functionality to the builtin flag library of
go, but provides much more functionality and nicer formatting. From the
documentation:

Package flags provides an extensive command line option parser.
The flags package is similar in functionality to the go builtin flag package
but provides more options and uses reflection to provide a convenient and
succinct way of specifying command line options.

Supported features:
* Options with short names (-v)
* Options with long names (--verbose)
* Options with and without arguments (bool v.s. other type)
* Options with optional arguments and default values
* Multiple option groups each containing a set of options
* Generate and print well-formatted help message
* Passing remaining command line arguments after -- (optional)
* Ignoring unknown command line options (optional)
* Supports -I/usr/include -I=/usr/include -I /usr/include option argument specification
* Supports multiple short options -aux
* Supports all primitive go types (string, int{8..64}, uint{8..64}, float)
* Supports same option multiple times (can store in slice or last option counts)
* Supports maps
* Supports function callbacks

The flags package uses structs, reflection and struct field tags
to allow users to specify command line options. This results in very simple
and consise specification of your application options. For example:

    type Options struct {
        Verbose []bool `short:"v" long:"verbose" description:"Show verbose debug information"`
    }

This specifies one option with a short name -v and a long name --verbose.
When either -v or --verbose is found on the command line, a 'true' value
will be appended to the Verbose field. e.g. when specifying -vvv, the
resulting value of Verbose will be {[true, true, true]}.

Example:
--------
	var opts struct {
		// Slice of bool will append 'true' each time the option
		// is encountered (can be set multiple times, like -vvv)
		Verbose []bool `short:"v" long:"verbose" description:"Show verbose debug information"`

		// Example of automatic marshalling to desired type (uint)
		Offset uint `long:"offset" description:"Offset"`

		// Example of a callback, called each time the option is found.
		Call func(string) `short:"c" description:"Call phone number"`

		// Example of a required flag
		Name string `short:"n" long:"name" description:"A name" required:"true"`

		// Example of a value name
		File string `short:"f" long:"file" description:"A file" value-name:"FILE"`

		// Example of a pointer
		Ptr *int `short:"p" description:"A pointer to an integer"`

		// Example of a slice of strings
		StringSlice []string `short:"s" description:"A slice of strings"`

		// Example of a slice of pointers
		PtrSlice []*string `long:"ptrslice" description:"A slice of pointers to string"`

		// Example of a map
		IntMap map[string]int `long:"intmap" description:"A map from string to int"`
	}

	// Callback which will invoke callto:<argument> to call a number.
	// Note that this works just on OS X (and probably only with
	// Skype) but it shows the idea.
	opts.Call = func(num string) {
		cmd := exec.Command("open", "callto:"+num)
		cmd.Start()
		cmd.Process.Release()
	}

	// Make some fake arguments to parse.
	args := []string{
		"-vv",
		"--offset=5",
		"-n", "Me",
		"-p", "3",
		"-s", "hello",
		"-s", "world",
		"--ptrslice", "hello",
		"--ptrslice", "world",
		"--intmap", "a:1",
		"--intmap", "b:5",
		"arg1",
		"arg2",
		"arg3",
	}

	// Parse flags from `args'. Note that here we use flags.ParseArgs for
	// the sake of making a working example. Normally, you would simply use
	// flags.Parse(&opts) which uses os.Args
	args, err := flags.ParseArgs(&opts, args)

	if err != nil {
		panic(err)
		os.Exit(1)
	}

	fmt.Printf("Verbosity: %v\n", opts.Verbose)
	fmt.Printf("Offset: %d\n", opts.Offset)
	fmt.Printf("Name: %s\n", opts.Name)
	fmt.Printf("Ptr: %d\n", *opts.Ptr)
	fmt.Printf("StringSlice: %v\n", opts.StringSlice)
	fmt.Printf("PtrSlice: [%v %v]\n", *opts.PtrSlice[0], *opts.PtrSlice[1])
	fmt.Printf("IntMap: [a:%v b:%v]\n", opts.IntMap["a"], opts.IntMap["b"])
	fmt.Printf("Remaining args: %s\n", strings.Join(args, " "))

	// Output: Verbosity: [true true]
	// Offset: 5
	// Name: Me
	// Ptr: 3
	// StringSlice: [hello world]
	// PtrSlice: [hello world]
	// IntMap: [a:1 b:5]
	// Remaining args: arg1 arg2 arg3

More information can be found in the godocs: <http://godoc.org/github.com/jessevdk/go-flags>
