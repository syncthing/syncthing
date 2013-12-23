package flags

import (
	"reflect"
	"unicode/utf8"
	"unsafe"
)

type scanHandler func(reflect.Value, *reflect.StructField) (bool, error)

func newGroup(shortDescription string, longDescription string, data interface{}) *Group {
	return &Group{
		ShortDescription: shortDescription,
		LongDescription:  longDescription,

		data: data,
	}
}

func (g *Group) optionByName(name string, namematch func(*Option, string) bool) *Option {
	prio := 0
	var retopt *Option

	for _, opt := range g.options {
		if namematch != nil && namematch(opt, name) && prio < 4 {
			retopt = opt
			prio = 4
		}

		if name == opt.field.Name && prio < 3 {
			retopt = opt
			prio = 3
		}

		if name == opt.LongName && prio < 2 {
			retopt = opt
			prio = 2
		}

		if opt.ShortName != 0 && name == string(opt.ShortName) && prio < 1 {
			retopt = opt
			prio = 1
		}
	}

	return retopt
}

func (g *Group) storeDefaults() {
	for _, option := range g.options {
		// First. empty out the value
		if len(option.Default) > 0 {
			option.clear()
		}

		for _, d := range option.Default {
			option.set(&d)
		}

		if !option.value.CanSet() {
			continue
		}

		option.defaultValue = reflect.ValueOf(option.value.Interface())
	}
}

func (g *Group) eachGroup(f func(*Group)) {
	f(g)

	for _, gg := range g.groups {
		gg.eachGroup(f)
	}
}

func (g *Group) scanStruct(realval reflect.Value, sfield *reflect.StructField, handler scanHandler) error {
	stype := realval.Type()

	if sfield != nil {
		if ok, err := handler(realval, sfield); err != nil {
			return err
		} else if ok {
			return nil
		}
	}

	for i := 0; i < stype.NumField(); i++ {
		field := stype.Field(i)

		// PkgName is set only for non-exported fields, which we ignore
		if field.PkgPath != "" {
			continue
		}

		mtag := newMultiTag(string(field.Tag))

		if err := mtag.Parse(); err != nil {
			return err
		}

		// Skip fields with the no-flag tag
		if mtag.Get("no-flag") != "" {
			continue
		}

		// Dive deep into structs or pointers to structs
		kind := field.Type.Kind()
		fld := realval.Field(i)

		if kind == reflect.Struct {
			if err := g.scanStruct(fld, &field, handler); err != nil {
				return err
			}
		} else if kind == reflect.Ptr && field.Type.Elem().Kind() == reflect.Struct {
			if fld.IsNil() {
				fld.Set(reflect.New(fld.Type().Elem()))
			}

			if err := g.scanStruct(reflect.Indirect(fld), &field, handler); err != nil {
				return err
			}
		}

		longname := mtag.Get("long")
		shortname := mtag.Get("short")

		// Need at least either a short or long name
		if longname == "" && shortname == "" && mtag.Get("ini-name") == "" {
			continue
		}

		short := rune(0)
		rc := utf8.RuneCountInString(shortname)

		if rc > 1 {
			return newErrorf(ErrShortNameTooLong,
				"short names can only be 1 character long, not `%s'",
				shortname)

		} else if rc == 1 {
			short, _ = utf8.DecodeRuneInString(shortname)
		}

		description := mtag.Get("description")
		def := mtag.GetMany("default")
		optionalValue := mtag.GetMany("optional-value")
		valueName := mtag.Get("value-name")
		defaultMask := mtag.Get("default-mask")

		optional := (mtag.Get("optional") != "")
		required := (mtag.Get("required") != "")

		option := &Option{
			Description:      description,
			ShortName:        short,
			LongName:         longname,
			Default:          def,
			OptionalArgument: optional,
			OptionalValue:    optionalValue,
			Required:         required,
			ValueName:        valueName,
			DefaultMask:      defaultMask,

			field: field,
			value: realval.Field(i),
			tag:   mtag,
		}

		g.options = append(g.options, option)
	}

	return nil
}

func (g *Group) checkForDuplicateFlags() *Error {
	shortNames := make(map[rune]*Option)
	longNames := make(map[string]*Option)

	var duplicateError *Error

	g.eachGroup(func(g *Group) {
		for _, option := range g.options {
			if option.LongName != "" {
				if otherOption, ok := longNames[option.LongName]; ok {
					duplicateError = newErrorf(ErrDuplicatedFlag, "option `%s' uses the same long name as option `%s'", option, otherOption)
					return
				}
				longNames[option.LongName] = option
			}
			if option.ShortName != 0 {
				if otherOption, ok := shortNames[option.ShortName]; ok {
					duplicateError = newErrorf(ErrDuplicatedFlag, "option `%s' uses the same short name as option `%s'", option, otherOption)
					return
				}
				shortNames[option.ShortName] = option
			}
		}
	})

	return duplicateError
}

func (g *Group) scanSubGroupHandler(realval reflect.Value, sfield *reflect.StructField) (bool, error) {
	mtag := newMultiTag(string(sfield.Tag))

	if err := mtag.Parse(); err != nil {
		return true, err
	}

	subgroup := mtag.Get("group")

	if len(subgroup) != 0 {
		ptrval := reflect.NewAt(realval.Type(), unsafe.Pointer(realval.UnsafeAddr()))
		description := mtag.Get("description")

		if _, err := g.AddGroup(subgroup, description, ptrval.Interface()); err != nil {
			return true, err
		}

		return true, nil
	}

	return false, nil
}

func (g *Group) scanType(handler scanHandler) error {
	// Get all the public fields in the data struct
	ptrval := reflect.ValueOf(g.data)

	if ptrval.Type().Kind() != reflect.Ptr {
		panic(ErrNotPointerToStruct)
	}

	stype := ptrval.Type().Elem()

	if stype.Kind() != reflect.Struct {
		panic(ErrNotPointerToStruct)
	}

	realval := reflect.Indirect(ptrval)

	if err := g.scanStruct(realval, nil, handler); err != nil {
		return err
	}

	if err := g.checkForDuplicateFlags(); err != nil {
		return err
	}

	return nil
}

func (g *Group) scan() error {
	return g.scanType(g.scanSubGroupHandler)
}

func (g *Group) groupByName(name string) *Group {
	if len(name) == 0 {
		return g
	}

	return g.Find(name)
}
