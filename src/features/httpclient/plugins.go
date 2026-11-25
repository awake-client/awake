package httpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"

	"github.com/awake-client/awake/src/domain"
	"github.com/awake-client/awake/src/shared/variables"
)

// ErrPluginsUnavailable is returned for a request using an authentication
// from a plugin when no plugin host is connected.
var ErrPluginsUnavailable = errors.New("httpclient: plugins cannot run in this build")

// PluginHost runs the plugins of a workspace while a request is sent. The
// application layer supplies the implementation so this feature does not
// depend on the plugins feature. Hooks report their failures in the returned
// logs and never stop a request; an authentication failure does.
type PluginHost interface {
	BeforeRequest(workspaceRoot string, request *domain.Request, variables map[string]string, secretNames []string) []string
	AfterResponse(workspaceRoot string, request domain.Request, response domain.ScriptResponse, variables map[string]string, secretNames []string) []string
	// DynamicValues computes the {{$name}} placeholders plugins provide,
	// leaving out the names none of them knows.
	DynamicValues(workspaceRoot string, names []string) (map[string]string, []string, error)
	Authenticate(workspaceRoot string, pluginID string, authenticationType string, httpRequest *http.Request, fields map[string]string) ([]string, error)
}

// SetPluginHost lets the plugins of the workspace take part in sending
// requests. It is a package function rather than a service method, so the
// host never crosses the Wails bridge.
func SetPluginHost(service *Service, host PluginHost) {
	service.plugins = host
}

// runPluginsBeforeRequest runs the beforeRequest hooks before the pre-request
// script. The variables they set are added as the highest layer.
func (service *Service) runPluginsBeforeRequest(
	workspaceRoot string,
	request *domain.Request,
	labeledLayers []variables.Layer,
	layers []map[string]string,
) ([]map[string]string, []string) {
	if service.plugins == nil {
		return layers, nil
	}
	merged := mergeLayers(layers)
	before := maps.Clone(merged)
	logs := service.plugins.BeforeRequest(workspaceRoot, request, merged, secretNames(labeledLayers))
	return withChangedValues(layers, before, merged), logs
}

// runPluginsAfterResponse runs the afterResponse hooks before the
// post-response script, which sees the variables they set.
func (service *Service) runPluginsAfterResponse(
	workspaceRoot string,
	request domain.Request,
	response Response,
	labeledLayers []variables.Layer,
	layers []map[string]string,
) ([]map[string]string, []string) {
	if service.plugins == nil {
		return layers, nil
	}
	merged := mergeLayers(layers)
	before := maps.Clone(merged)
	logs := service.plugins.AfterResponse(workspaceRoot, request, scriptResponse(response), merged, secretNames(labeledLayers))
	return withChangedValues(layers, before, merged), logs
}

// addPluginDynamicValues computes the {{$name}} placeholders of the request
// that are not built in, once per request, and adds them as a layer.
func (service *Service) addPluginDynamicValues(workspaceRoot string, request domain.Request, layers []map[string]string) ([]map[string]string, []string, error) {
	if service.plugins == nil {
		return layers, nil, nil
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return nil, nil, fmt.Errorf("read the placeholders of the request: %w", err)
	}
	names := variables.ExternalDynamicNames(string(encoded))
	if len(names) == 0 {
		return layers, nil, nil
	}
	values, logs, err := service.plugins.DynamicValues(workspaceRoot, names)
	if err != nil {
		return nil, logs, err
	}
	return append(layers, values), logs, nil
}

// buildRequest builds the HTTP request, then lets the plugin that provides
// its authentication sign it, since a signature covers the final URL,
// headers and body.
func (service *Service) buildRequest(
	parent context.Context,
	workspaceRoot string,
	request domain.Request,
	layers []map[string]string,
	oauth2Token string,
) (*http.Request, []string, error) {
	httpRequest, err := buildHTTPRequestWithContext(parent, request, layers, oauth2Token)
	if err != nil {
		return nil, nil, err
	}
	authentication := resolvedAuthentication(request.Authentication)
	pluginID, typeName, isPlugin := authentication.Type.PluginContribution()
	if !isPlugin {
		return httpRequest, nil, nil
	}
	if service.plugins == nil {
		return nil, nil, ErrPluginsUnavailable
	}

	fields := make(map[string]string, len(authentication.Fields))
	for key, value := range authentication.Fields {
		resolved, err := interpolate(value, layers)
		if err != nil {
			return nil, nil, err
		}
		fields[key] = resolved
	}
	logs, err := service.plugins.Authenticate(workspaceRoot, pluginID, typeName, httpRequest, fields)
	if err != nil {
		return nil, logs, fmt.Errorf("authenticate with %s: %w", pluginID, err)
	}
	return httpRequest, logs, nil
}

// secretNames lists the variables read from the keychain, which plugins only
// see with the secrets permission.
func secretNames(labeledLayers []variables.Layer) []string {
	names := []string{}
	for _, layer := range labeledLayers {
		if !layer.Secret {
			continue
		}
		for name := range layer.Values {
			names = append(names, name)
		}
	}
	return names
}

// withChangedValues adds the variables a plugin set or changed as the highest
// layer. A variable a plugin deletes keeps its value from the lower layers.
func withChangedValues(layers []map[string]string, before map[string]string, after map[string]string) []map[string]string {
	changed := map[string]string{}
	for name, value := range after {
		if previous, found := before[name]; !found || previous != value {
			changed[name] = value
		}
	}
	if len(changed) == 0 {
		return layers
	}
	return append(layers, changed)
}

func scriptResponse(response Response) domain.ScriptResponse {
	return domain.ScriptResponse{
		StatusCode:           response.StatusCode,
		Status:               response.Status,
		Headers:              response.Headers,
		Body:                 response.Preview,
		Encoding:             response.Encoding,
		DurationMilliseconds: response.DurationMilliseconds,
	}
}

// withPluginLogs puts the logs of plugins before those of the script, in the
// order they ran.
func withPluginLogs(outcome domain.ScriptOutcome, logs ...[]string) domain.ScriptOutcome {
	combined := []string{}
	for _, lines := range logs {
		combined = append(combined, lines...)
	}
	if len(combined) == 0 {
		return outcome
	}
	outcome.Logs = append(combined, outcome.Logs...)
	return outcome
}
