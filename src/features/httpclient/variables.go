package httpclient

import (
	"sort"

	"github.com/clementherve/awake/src/shared/variables"
)

// DynamicVariable describes a {{$name}} placeholder generated at send time.
type DynamicVariable struct {
	Name        string
	Description string
}

// DynamicVariables lists the placeholders, such as {{$uuid}}, that receive a
// fresh value each time a request is sent.
func (service *Service) DynamicVariables() []DynamicVariable {
	builtIn := variables.DynamicVariables()
	result := make([]DynamicVariable, 0, len(builtIn))
	for _, variable := range builtIn {
		result = append(result, DynamicVariable{Name: variable.Name, Description: variable.Description})
	}
	return result
}

// ResolvedVariable is the value a request sees for one variable, after every
// layer has been applied. Secret values are never returned.
type ResolvedVariable struct {
	Name   string
	Value  string
	Source string
	Secret bool
}

// ResolveVariables lists the variables visible to a request with the given
// environment, as the interface shows them while editing.
func (service *Service) ResolveVariables(workspaceRoot string, requestPath string, environmentFileName string) ([]ResolvedVariable, error) {
	layers, err := service.source.LoadVariableLayers(workspaceRoot, requestPath, environmentFileName)
	if err != nil {
		return nil, err
	}

	merged := make(map[string]ResolvedVariable)
	for _, layer := range layers {
		for name, value := range layer.Values {
			resolved := ResolvedVariable{Name: name, Value: value, Source: layer.Source, Secret: layer.Secret}
			if layer.Secret {
				resolved.Value = ""
			}
			merged[name] = resolved
		}
	}

	result := make([]ResolvedVariable, 0, len(merged))
	for _, resolved := range merged {
		result = append(result, resolved)
	}
	sort.Slice(result, func(first int, second int) bool { return result[first].Name < result[second].Name })
	return result, nil
}
