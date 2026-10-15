package service

import (
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// ApplyErrorStatusMapping attaches a host-configured response override to
// newApiErr when an enabled mapping rule matches its status code and message.
// It never changes the original status code, error or any other field.
func ApplyErrorStatusMapping(newApiErr *types.NewAPIError) {
	if newApiErr == nil {
		return
	}
	output := operation_setting.MatchErrorStatusMapping(newApiErr.StatusCode, newApiErr.Error())
	if output == nil {
		return
	}
	headers := make(map[string]string, len(output.Headers))
	for _, header := range output.Headers {
		headers[header.Key] = header.Value
	}
	newApiErr.ResponseOverride = &types.ErrorResponseOverride{
		StatusCode: output.StatusCode,
		Headers:    headers,
		Message:    output.Message,
		Type:       output.Type,
		Code:       output.Code,
	}
}
