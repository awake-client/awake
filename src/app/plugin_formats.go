package app

import (
	"github.com/awake-client/awake/src/features/interop"
	"github.com/awake-client/awake/src/features/plugins"
)

// pluginFormats connects the interop feature to the formats of the enabled
// plugins.
type pluginFormats struct {
	host *plugins.Host
}

func (adapter pluginFormats) Formats(workspaceRoot string) ([]interop.PluginFormat, []interop.PluginFormat) {
	imports, exports := adapter.host.Formats(workspaceRoot)
	return interopFormats(imports), interopFormats(exports)
}

func (adapter pluginFormats) Import(workspaceRoot string, format interop.Format, fileName string, content []byte) ([]byte, error) {
	return adapter.host.Import(workspaceRoot, string(format), fileName, content)
}

func (adapter pluginFormats) Export(workspaceRoot string, format interop.Format, collections []byte) (string, error) {
	return adapter.host.Export(workspaceRoot, string(format), collections)
}

func interopFormats(formats []plugins.FormatInfo) []interop.PluginFormat {
	converted := make([]interop.PluginFormat, 0, len(formats))
	for _, format := range formats {
		converted = append(converted, interop.PluginFormat{
			Format:     interop.Format(format.Format),
			Label:      format.Label,
			Extensions: format.Extensions,
		})
	}
	return converted
}
