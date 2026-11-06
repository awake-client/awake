package domain

// Colors lists the names a workspace, a collection or an environment may use
// as its color. The interface maps each name to a shade that suits the
// light and dark themes, so files store names rather than hexadecimal codes.
var Colors = []string{"orange", "amber", "green", "teal", "blue", "violet", "pink", "red", "gray"}

// IsColor reports whether name is empty, meaning no color, or a known color.
func IsColor(name string) bool {
	if name == "" {
		return true
	}
	for _, color := range Colors {
		if color == name {
			return true
		}
	}
	return false
}
