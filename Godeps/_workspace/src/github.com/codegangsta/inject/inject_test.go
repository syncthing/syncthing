package inject_test

import (
	"github.com/codegangsta/inject"
	"reflect"
	"testing"
)

type SpecialString interface {
}

type TestStruct struct {
	Dep1 string        `inject`
	Dep2 SpecialString `inject`
	Dep3 string
}

/* Test Helpers */
func expect(t *testing.T, a interface{}, b interface{}) {
	if a != b {
		t.Errorf("Expected %v (type %v) - Got %v (type %v)", b, reflect.TypeOf(b), a, reflect.TypeOf(a))
	}
}

func refute(t *testing.T, a interface{}, b interface{}) {
	if a == b {
		t.Errorf("Did not expect %v (type %v) - Got %v (type %v)", b, reflect.TypeOf(b), a, reflect.TypeOf(a))
	}
}

func Test_InjectorInvoke(t *testing.T) {
	injector := inject.New()
	expect(t, injector == nil, false)

	dep := "some dependency"
	injector.Map(dep)
	dep2 := "another dep"
	injector.MapTo(dep2, (*SpecialString)(nil))

	_, err := injector.Invoke(func(d1 string, d2 SpecialString) {
		expect(t, d1, dep)
		expect(t, d2, dep2)
	})

	expect(t, err, nil)
}

func Test_InjectorInvokeReturnValues(t *testing.T) {
	injector := inject.New()
	expect(t, injector == nil, false)

	dep := "some dependency"
	injector.Map(dep)
	dep2 := "another dep"
	injector.MapTo(dep2, (*SpecialString)(nil))

	result, err := injector.Invoke(func(d1 string, d2 SpecialString) string {
		expect(t, d1, dep)
		expect(t, d2, dep2)
		return "Hello world"
	})

	expect(t, result[0].String(), "Hello world")
	expect(t, err, nil)
}

func Test_InjectorApply(t *testing.T) {
	injector := inject.New()

	injector.Map("a dep").MapTo("another dep", (*SpecialString)(nil))

	s := TestStruct{}
	err := injector.Apply(&s)
	expect(t, err, nil)

	expect(t, s.Dep1, "a dep")
	expect(t, s.Dep2, "another dep")
}

func Test_InterfaceOf(t *testing.T) {
	iType := inject.InterfaceOf((*SpecialString)(nil))
	expect(t, iType.Kind(), reflect.Interface)

	iType = inject.InterfaceOf((**SpecialString)(nil))
	expect(t, iType.Kind(), reflect.Interface)

	// Expecting nil
	defer func() {
		rec := recover()
		refute(t, rec, nil)
	}()
	iType = inject.InterfaceOf((*testing.T)(nil))
}

func Test_InjectorGet(t *testing.T) {
	injector := inject.New()

	injector.Map("some dependency")

	expect(t, injector.Get(reflect.TypeOf("string")).IsValid(), true)
	expect(t, injector.Get(reflect.TypeOf(11)).IsValid(), false)
}

func Test_InjectorSetParent(t *testing.T) {
	injector := inject.New()
	injector.MapTo("another dep", (*SpecialString)(nil))

	injector2 := inject.New()
	injector2.SetParent(injector)

	expect(t, injector2.Get(inject.InterfaceOf((*SpecialString)(nil))).IsValid(), true)
}
