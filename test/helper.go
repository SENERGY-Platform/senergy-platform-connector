package test

import (
	"encoding/json"
	"errors"
	"github.com/SENERGY-Platform/converter/lib/converter/characteristics"
	"github.com/SENERGY-Platform/platform-connector-lib/iot"
	"github.com/SENERGY-Platform/platform-connector-lib/model"
	"github.com/SENERGY-Platform/platform-connector-lib/security"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"time"
)

func createTestCommandMsg(config configuration.Config, deviceUri string, serviceUri string, msg map[string]interface{}) (result model.ProtocolMsg, err error) {
	sec, err := security.New(config.AuthEndpoint, config.AuthClientId, config.AuthClientSecret, config.JwtIssuer, config.JwtPrivateKey, config.JwtExpiration, config.AuthExpirationTimeBuffer, 0, []string{}, 0, 0)
	if err != nil {
		return result, err
	}
	token, err := sec.Access()
	if err != nil {
		return result, err
	}
	iot := iot.New(config.DeviceManagerUrl, config.DeviceRepoUrl, "")
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

func createOptimisticTestCommandMsg(config configuration.Config, deviceUri string, serviceUri string, msg map[string]interface{}) (result model.ProtocolMsg, err error) {
	sec, err := security.New(config.AuthEndpoint, config.AuthClientId, config.AuthClientSecret, config.JwtIssuer, config.JwtPrivateKey, config.JwtExpiration, config.AuthExpirationTimeBuffer, 0, []string{}, 0, 0)
	if err != nil {
		return result, err
	}
	token, err := sec.Access()
	if err != nil {
		return result, err
	}
	iot := iot.New(config.DeviceManagerUrl, config.DeviceRepoUrl, "")
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

const adminjwt = security.JwtToken("Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJqdGkiOiJiOGUyNGZkNy1jNjJlLTRhNWQtOTQ4ZC1mZGI2ZWVkM2JmYzYiLCJleHAiOjE1MzA1MzIwMzIsIm5iZiI6MCwiaWF0IjoxNTMwNTI4NDMyLCJpc3MiOiJodHRwczovL2F1dGguc2VwbC5pbmZhaS5vcmcvYXV0aC9yZWFsbXMvbWFzdGVyIiwiYXVkIjoiZnJvbnRlbmQiLCJzdWIiOiJhZG1pbi1kZDY5ZWEwZC1mNTUzLTQzMzYtODBmMy03ZjQ1NjdmODVjN2IiLCJ0eXAiOiJCZWFyZXIiLCJhenAiOiJmcm9udGVuZCIsIm5vbmNlIjoiMjJlMGVjZjgtZjhhMS00NDQ1LWFmMjctNGQ1M2JmNWQxOGI5IiwiYXV0aF90aW1lIjoxNTMwNTI4NDIzLCJzZXNzaW9uX3N0YXRlIjoiMWQ3NWE5ODQtNzM1OS00MWJlLTgxYjktNzMyZDgyNzRjMjNlIiwiYWNyIjoiMCIsImFsbG93ZWQtb3JpZ2lucyI6WyIqIl0sInJlYWxtX2FjY2VzcyI6eyJyb2xlcyI6WyJjcmVhdGUtcmVhbG0iLCJhZG1pbiIsImRldmVsb3BlciIsInVtYV9hdXRob3JpemF0aW9uIiwidXNlciJdfSwicmVzb3VyY2VfYWNjZXNzIjp7Im1hc3Rlci1yZWFsbSI6eyJyb2xlcyI6WyJ2aWV3LWlkZW50aXR5LXByb3ZpZGVycyIsInZpZXctcmVhbG0iLCJtYW5hZ2UtaWRlbnRpdHktcHJvdmlkZXJzIiwiaW1wZXJzb25hdGlvbiIsImNyZWF0ZS1jbGllbnQiLCJtYW5hZ2UtdXNlcnMiLCJxdWVyeS1yZWFsbXMiLCJ2aWV3LWF1dGhvcml6YXRpb24iLCJxdWVyeS1jbGllbnRzIiwicXVlcnktdXNlcnMiLCJtYW5hZ2UtZXZlbnRzIiwibWFuYWdlLXJlYWxtIiwidmlldy1ldmVudHMiLCJ2aWV3LXVzZXJzIiwidmlldy1jbGllbnRzIiwibWFuYWdlLWF1dGhvcml6YXRpb24iLCJtYW5hZ2UtY2xpZW50cyIsInF1ZXJ5LWdyb3VwcyJdfSwiYWNjb3VudCI6eyJyb2xlcyI6WyJtYW5hZ2UtYWNjb3VudCIsIm1hbmFnZS1hY2NvdW50LWxpbmtzIiwidmlldy1wcm9maWxlIl19fSwicm9sZXMiOlsidW1hX2F1dGhvcml6YXRpb24iLCJhZG1pbiIsImNyZWF0ZS1yZWFsbSIsImRldmVsb3BlciIsInVzZXIiLCJvZmZsaW5lX2FjY2VzcyJdLCJuYW1lIjoiZGYgZGZmZmYiLCJwcmVmZXJyZWRfdXNlcm5hbWUiOiJzZXBsIiwiZ2l2ZW5fbmFtZSI6ImRmIiwiZmFtaWx5X25hbWUiOiJkZmZmZiIsImVtYWlsIjoic2VwbEBzZXBsLmRlIn0.p5x_Kp9tbpVGnjFMe-2VCjNTo407kA58moagOMR1bp8")

func createDeviceType(conf configuration.Config, managerUrl string, characteristicId string) (typeId string, serviceId1 string, getServiceTopic string, serviceId2 string, setServiceTopic string, err error) {
	protocol := model.Protocol{}
	err = adminjwt.PostJSON(managerUrl+"/protocols", model.Protocol{
		Name:             conf.Protocol,
		Handler:          conf.Protocol,
		ProtocolSegments: []model.ProtocolSegment{{Name: "metrics"}},
	}, &protocol)
	if err != nil {
		return "", "", "", "", "", err
	}

	time.Sleep(10 * time.Second)

	//{"level": 42, "title": "event", "updateTime": 0}
	devicetype := model.DeviceType{}
	err = adminjwt.PostJSON(managerUrl+"/device-types", model.DeviceType{
		Name: "foo",
		Services: []model.Service{
			{
				Name:        "sepl_get",
				LocalId:     "sepl_get",
				Description: "sepl_get",
				ProtocolId:  protocol.Id,
				Attributes: []model.Attribute{
					{
						Key:   "senergy/time_path",
						Value: "metrics.updateTime",
					},
				},
				Outputs: []model.Content{
					{
						ProtocolSegmentId: protocol.ProtocolSegments[0].Id,
						Serialization:     "json",
						ContentVariable: model.ContentVariable{
							Name: "metrics",
							Type: model.Structure,
							SubContentVariables: []model.ContentVariable{
								{
									Name:             "updateTime",
									Type:             model.Integer,
									CharacteristicId: characteristics.UnixSeconds,
								},
								{
									Name:             "level",
									Type:             model.Integer,
									CharacteristicId: characteristicId,
								},
								{
									Name:          "level_unit",
									Type:          model.String,
									UnitReference: "level",
								},
								{
									Name: "title",
									Type: model.String,
								},
								{
									Name: "missing",
									Type: model.String,
								},
							},
						},
					},
				},
			},
			{
				Name:        "exact",
				LocalId:     "exact",
				Description: "exact",
				ProtocolId:  protocol.Id,
				Inputs: []model.Content{
					{
						ProtocolSegmentId: protocol.ProtocolSegments[0].Id,
						Serialization:     "json",
						ContentVariable: model.ContentVariable{
							Name: "metrics",
							Type: model.Structure,
							SubContentVariables: []model.ContentVariable{
								{
									Name: "level",
									Type: model.Integer,
								},
							},
						},
					},
				},
			},
		},
	}, &devicetype)

	if err != nil {
		return "", "", "", "", "", err
	}
	time.Sleep(10 * time.Second)
	return devicetype.Id, devicetype.Services[0].Id, model.ServiceIdToTopic(devicetype.Services[0].Id), devicetype.Services[1].Id, model.ServiceIdToTopic(devicetype.Services[1].Id), nil
}

func createCharacteristic(name string, config configuration.Config) (string, error) {
	var char model.Characteristic
	err := adminjwt.PostJSON(config.DeviceManagerUrl+"/characteristics", &model.Characteristic{
		Name:               name,
		Type:               model.Float,
		MinValue:           0,
		MaxValue:           1,
		Value:              0,
		SubCharacteristics: nil,
	}, &char)
	return char.Id, err
}
