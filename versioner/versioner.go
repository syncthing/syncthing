package versioner

type Versioner interface {
	Archive(path string) error
}

var Factories = map[string]func(map[string]string) Versioner{}
