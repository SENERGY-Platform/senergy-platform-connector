package test

import (
	"encoding/json"
	"errors"
	"github.com/SENERGY-Platform/platform-connector-lib/iot"
	"github.com/SENERGY-Platform/platform-connector-lib/model"
	"github.com/SENERGY-Platform/platform-connector-lib/security"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib"
)

func createTestCommandMsg(config lib.Config, deviceUri string, serviceUri string, msg map[string]interface{}) (result model.ProtocolMsg, err error) {
	token, err := security.New(config.AuthEndpoint, config.AuthClientId, config.AuthClientSecret, config.JwtIssuer, config.JwtPrivateKey, config.JwtExpiration, config.AuthExpirationTimeBuffer, 0, []string{}).Access()
	if err != nil {
		return result, err
	}
	iot := iot.New(config.DeviceManagerUrl, config.DeviceRepoUrl)
	device, err := iot.GetDeviceByLocalId(deviceUri, token)
	if err != nil {
		return result, err
	}
	dt, err := iot.GetDeviceType(device.DeviceTypeId, token)
	if err != nil {
		return result, err
	}
	for _, service := range dt.Services {
		if service.LocalId == serviceUri {
			protocol, err := iot.GetProtocol(service.ProtocolId, token)
			if err != nil {
				return result, err
			}
			result := model.ProtocolMsg{
				Metadata: model.Metadata{
					Device:               device,
					Service:              service,
					Protocol:             protocol,
					InputCharacteristic:  "",
					OutputCharacteristic: "",
				},
			}

			if msg != nil {
				payload, err := json.Marshal(msg)
				if err != nil {
					return result, err
				}
				result.Request.Input = map[string]string{"metrics": string(payload)}
			}
			return result, nil
		}
	}

	return result, errors.New("unable to find device for command creation")
}

func createOptimisticTestCommandMsg(config lib.Config, deviceUri string, serviceUri string, msg map[string]interface{}) (result model.ProtocolMsg, err error) {
	token, err := security.New(config.AuthEndpoint, config.AuthClientId, config.AuthClientSecret, config.JwtIssuer, config.JwtPrivateKey, config.JwtExpiration, config.AuthExpirationTimeBuffer, 0, []string{}).Access()
	if err != nil {
		return result, err
	}
	iot := iot.New(config.DeviceManagerUrl, config.DeviceRepoUrl)
	device, err := iot.GetDeviceByLocalId(deviceUri, token)
	if err != nil {
		return result, err
	}
	dt, err := iot.GetDeviceType(device.DeviceTypeId, token)
	if err != nil {
		return result, err
	}
	for _, service := range dt.Services {
		if service.LocalId == serviceUri {
			protocol, err := iot.GetProtocol(service.ProtocolId, token)
			if err != nil {
				return result, err
			}
			result := model.ProtocolMsg{
				Metadata: model.Metadata{
					Device:               device,
					Service:              service,
					Protocol:             protocol,
					InputCharacteristic:  "",
					OutputCharacteristic: "",
				},
				TaskInfo: model.TaskInfo{CompletionStrategy: model.Optimistic},
			}

			if msg != nil {
				payload, err := json.Marshal(msg)
				if err != nil {
					return result, err
				}
				result.Request.Input = map[string]string{"metrics": string(payload)}
			}
			return result, nil
		}
	}

	return result, errors.New("unable to find device for command creation")
}
