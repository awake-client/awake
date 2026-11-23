package collections

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"

	"go.yaml.in/yaml/v3"

	"github.com/awake-client/awake/src/shared/files"
	"github.com/awake-client/awake/src/shared/yamlcodec"
)

const (
	// orderKey lists the file and directory names of a directory in the order
	// the tree shows them. Collections and folders keep it in their metadata
	// file; the collection roots keep it in orderFileName.
	orderKey = "order"
	// orderFileName holds the order of the collections of a root, such as
	// collections/order.yaml.
	orderFileName = "order.yaml"
)

// orderDocument is the content of a root order.yaml.
type orderDocument struct {
	Order []string `yaml:"order,omitempty"`
}

// isRootOrderFile reports whether a cleaned path is the order file of a
// collection root, which is never a request.
func isRootOrderFile(cleaned string) bool {
	return path.Base(cleaned) == orderFileName && isRoot(path.Dir(cleaned))
}

// orderDirectory returns the cleaned directory whose order a listing uses. An
// empty path stands for the shared collections root.
func orderDirectory(relativePath string) string {
	if relativePath == "" {
		return directoryName
	}
	return path.Clean(relativePath)
}

// orderFilePath returns the file that stores the order of a directory: the
// root order file, collection.yaml or folder.yaml.
func orderFilePath(directory string, cleaned string) string {
	switch {
	case isRoot(cleaned):
		return filepath.Join(directory, orderFileName)
	case isCollectionPath(cleaned):
		return filepath.Join(directory, collectionFileName)
	default:
		return filepath.Join(directory, folderFileName)
	}
}

// readOrder returns the names a directory lists in its order, empty when it
// keeps the default order.
func readOrder(directory string, cleaned string) ([]string, error) {
	content, err := os.ReadFile(orderFilePath(directory, cleaned))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var document orderDocument
	if err := yaml.Unmarshal(content, &document); err != nil {
		// A malformed metadata file is reported by the code that reads it;
		// the order alone falls back to the default.
		return nil, nil
	}
	return document.Order, nil
}

// writeOrder stores the order of a directory, rewriting only the order key so
// the comments and the other keys of the metadata file survive. A collection
// or folder without a metadata file receives one named after its directory.
func writeOrder(directory string, cleaned string, names []string) error {
	filePath := orderFilePath(directory, cleaned)
	existing, err := os.ReadFile(filePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read order of %s: %w", cleaned, err)
	}
	if len(existing) == 0 {
		if isRoot(cleaned) {
			existing = []byte("{}\n")
		} else {
			existing, err = yamlcodec.EncodeValue(folderMetadata{Name: filepath.Base(directory)})
			if err != nil {
				return fmt.Errorf("encode metadata of %s: %w", cleaned, err)
			}
		}
	}

	content, err := encodeOrder(existing, names)
	if err != nil {
		return fmt.Errorf("encode order of %s: %w", cleaned, err)
	}
	if isRoot(cleaned) && len(names) == 0 {
		if err := os.Remove(filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("delete order of %s: %w", cleaned, err)
		}
		return nil
	}
	return files.WriteAtomically(filePath, content)
}

// encodeOrder sets the order key of a YAML mapping, or removes it when names
// is empty.
func encodeOrder(existing []byte, names []string) ([]byte, error) {
	document, err := yamlcodec.Parse(existing)
	if err != nil {
		return nil, err
	}
	if len(document.Root().Content) == 0 || document.Root().Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("collections: metadata file is not a mapping")
	}
	mapping := document.Root().Content[0]
	// An empty document parsed from "{}" keeps the flow style otherwise.
	mapping.Style = 0
	yamlcodec.SetStringSequence(mapping, orderKey, names)
	return document.Encode()
}

// keepOrder copies the order of a metadata file that is about to be replaced
// into its new content, so saving the settings never resets the order.
func keepOrder(filePath string, content []byte) ([]byte, error) {
	existing, err := os.ReadFile(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return content, nil
		}
		return nil, err
	}
	var document orderDocument
	if err := yaml.Unmarshal(existing, &document); err != nil || len(document.Order) == 0 {
		return content, nil
	}
	return encodeOrder(content, document.Order)
}

// sortByOrder puts the entries a directory lists in its order first, in that
// order. The others follow in the default order, so a new entry shows up at
// the end of an ordered directory.
func sortByOrder(entries []Entry, order []string) {
	if len(order) == 0 {
		return
	}
	rank := make(map[string]int, len(order))
	for index, name := range order {
		if _, exists := rank[name]; !exists {
			rank[name] = index
		}
	}
	position := func(entry Entry) int {
		if index, ok := rank[path.Base(entry.Path)]; ok {
			return index
		}
		return len(order)
	}
	sort.SliceStable(entries, func(first int, second int) bool {
		return position(entries[first]) < position(entries[second])
	})
}

// forgetInOrder removes an entry from the order of its directory, after it
// moved away or was deleted.
func (repository *Repository) forgetInOrder(cleaned string) error {
	parent := path.Dir(cleaned)
	directory, err := repository.resolve(parent)
	if err != nil {
		return err
	}
	order, err := readOrder(directory, parent)
	if err != nil {
		return fmt.Errorf("read order of %s: %w", parent, err)
	}
	name := path.Base(cleaned)
	if !slices.Contains(order, name) {
		return nil
	}
	return writeOrder(directory, parent, slices.DeleteFunc(order, func(candidate string) bool {
		return candidate == name
	}))
}

// appendToOrder adds an entry that moved into an ordered directory at its
// end, where the tree already shows it.
func (repository *Repository) appendToOrder(cleaned string) error {
	parent := path.Dir(cleaned)
	directory, err := repository.resolve(parent)
	if err != nil {
		return err
	}
	order, err := readOrder(directory, parent)
	if err != nil {
		return fmt.Errorf("read order of %s: %w", parent, err)
	}
	name := path.Base(cleaned)
	if len(order) == 0 || slices.Contains(order, name) {
		return nil
	}
	return writeOrder(directory, parent, append(order, name))
}

// Arrange places a request, a folder or a collection just before or just
// after another entry, and returns its new path. A request or folder placed
// next to an entry of another directory moves into that directory first. A
// collection stays in its root, among the other collections.
func (repository *Repository) Arrange(relativePath string, anchorPath string, after bool) (string, error) {
	source := path.Clean(relativePath)
	anchor := path.Clean(anchorPath)
	if source == "." || anchor == "." || isRoot(source) || isRoot(anchor) {
		return "", ErrInvalidPath
	}
	if isCollectionPath(source) != isCollectionPath(anchor) {
		return "", ErrInvalidPath
	}
	if isCollectionPath(source) && path.Dir(source) != path.Dir(anchor) {
		return "", ErrInvalidPath
	}
	if isRootOrderFile(source) || isRootOrderFile(anchor) {
		return "", ErrInvalidPath
	}

	anchorFile, err := repository.resolve(anchor)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(anchorFile); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrEntryNotFound
		}
		return "", fmt.Errorf("inspect %s: %w", anchorPath, err)
	}
	if source == anchor {
		return source, nil
	}

	parent := path.Dir(anchor)
	if path.Dir(source) != parent {
		moved, err := repository.Move(source, parent)
		if err != nil {
			return "", err
		}
		source = moved
	}

	entries, err := repository.List(parent)
	if err != nil {
		return "", err
	}
	name := path.Base(source)
	anchorName := path.Base(anchor)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		entryName := path.Base(entry.Path)
		if entryName == name {
			continue
		}
		if entryName == anchorName && !after {
			names = append(names, name)
		}
		names = append(names, entryName)
		if entryName == anchorName && after {
			names = append(names, name)
		}
	}

	directory, err := repository.resolve(parent)
	if err != nil {
		return "", err
	}
	if err := writeOrder(directory, parent, names); err != nil {
		return "", err
	}
	return source, nil
}
