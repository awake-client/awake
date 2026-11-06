package httpclient

import "github.com/clementherve/awake/src/shared/variables"

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
