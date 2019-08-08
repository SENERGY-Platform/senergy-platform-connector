package test

import (
	"encoding/json"
	"errors"
	"github.com/SENERGY-Platform/platform-connector-lib/iot"
	"github.com/SENERGY-Platform/platform-connector-lib/model"
	"github.com/SENERGY-Platform/platform-connector-lib/security"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib"
)

func createTestCommandMsg(config lib.Config, deviceUri string, serviceUri string, msg map[string]interface{}) (result model.Envelope, err error) {
	token, err := security.New(config.AuthEndpoint, config.AuthClientId, config.AuthClientSecret, config.JwtIssuer, config.JwtPrivateKey, config.JwtExpiration, config.AuthExpirationTimeBuffer, 0, []string{}).Access()
	if err != nil {
		return result, err
	}
	iot := iot.New(config.IotRepoUrl, config.DeviceRepoUrl, "")
	device, err := iot.DeviceUrlToIotDevice(deviceUri, token)
	if err != nil {
		return result, err
	}
	dt, err := iot.GetDeviceType(device.DeviceType, token)

	found := false
	for _, service := range dt.Services {
		if service.Url == serviceUri {
			found = true
			result.ServiceId = service.Id
			result.DeviceId = device.Id
			value := model.ProtocolMsg{
				Service:          service,
				ServiceId:        service.Id,
				ServiceUrl:       serviceUri,
				DeviceUrl:        deviceUri,
				DeviceInstanceId: device.Id,
				OutputName:       "result",
				TaskId:           "",
				WorkerId:         "",
			}

			if msg != nil {
				payload, err := json.Marshal(msg)
				if err != nil {
					return result, err
				}
				value.ProtocolParts = []model.ProtocolPart{{Value: string(payload), Name: "metrics"}}
			}

			result.Value = value

		}
	}

	if !found {
		err = errors.New("unable to find device for command creation")
	}

	return
}

func createOptimisticTestCommandMsg(config lib.Config, deviceUri string, serviceUri string, msg map[string]interface{}) (result model.Envelope, err error) {
	token, err := security.New(config.AuthEndpoint, config.AuthClientId, config.AuthClientSecret, config.JwtIssuer, config.JwtPrivateKey, config.JwtExpiration, config.AuthExpirationTimeBuffer, 0, []string{}).Access()
	if err != nil {
		return result, err
	}
	iot := iot.New(config.IotRepoUrl, config.DeviceRepoUrl, "")
	device, err := iot.DeviceUrlToIotDevice(deviceUri, token)
	if err != nil {
		return result, err
	}
	dt, err := iot.GetDeviceType(device.DeviceType, token)

	found := false
	for _, service := range dt.Services {
		if service.Url == serviceUri {
			found = true
			result.ServiceId = service.Id
			result.DeviceId = device.Id
			value := model.ProtocolMsg{
				Service:            service,
				ServiceId:          service.Id,
				ServiceUrl:         serviceUri,
				DeviceUrl:          deviceUri,
				DeviceInstanceId:   device.Id,
				OutputName:         "result",
				TaskId:             "",
				WorkerId:           "",
				CompletionStrategy: model.Optimistic,
			}

			if msg != nil {
				payload, err := json.Marshal(msg)
				if err != nil {
					return result, err
				}
				value.ProtocolParts = []model.ProtocolPart{{Value: string(payload), Name: "metrics"}}
			}

			result.Value = value

		}
	}

	if !found {
		err = errors.New("unable to find device for command creation")
	}

	return
}
