package resource

type Provider interface {
	NewInstance(string) (Instance, error)
}

// TranslatedProvider manipulates resource locations, so as to allow
// sandboxing, or relative paths for example.
type TranslatedProvider struct {
	// The underlying Provider.
	BaseProvider Provider
	// Some location used in calculating final locations.
	BaseLocation string
	// Function that takes BaseLocation, and the caller location and returns
	// the location to be used with the BaseProvider.
	JoinLocations func(base, rel string) string
}

func (me *TranslatedProvider) NewInstance(rel string) (Instance, error) {
	return me.BaseProvider.NewInstance(me.JoinLocations(me.BaseLocation, rel))
}
