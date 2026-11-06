package variables

// Layer is one source of variables, such as the workspace, a collection or an
// environment. Layers are applied in order, the last one winning.
type Layer struct {
	// Source names where the values come from, for display.
	Source string
	Values map[string]string
	// Secret marks values read from the keychain, never shown in clear.
	Secret bool
}

// Maps returns the values of each layer, in order, for interpolation.
func Maps(layers []Layer) []map[string]string {
	maps := make([]map[string]string, 0, len(layers))
	for _, layer := range layers {
		maps = append(maps, layer.Values)
	}
	return maps
}

// Unlabeled wraps plain maps into layers without a source, mostly for tests.
func Unlabeled(maps ...map[string]string) []Layer {
	layers := make([]Layer, 0, len(maps))
	for _, values := range maps {
		layers = append(layers, Layer{Values: values})
	}
	return layers
}
