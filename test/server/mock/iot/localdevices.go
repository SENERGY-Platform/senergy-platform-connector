package iot

import (
	"encoding/json"
	"github.com/SENERGY-Platform/platform-connector-lib/model"
	"github.com/SENERGY-Platform/service-commons/pkg/jwt"
	"github.com/julienschmidt/httprouter"
	"log"
	"net/http"
)

func LocalDevicesEndpoints(control *Controller, router *httprouter.Router) {
	resource := "/local-devices"

	router.GET(resource+"/:id", func(writer http.ResponseWriter, request *http.Request, params httprouter.Params) {
		id := params.ByName("id")
		id, err, errCode := control.DeviceLocalIdToId(id)
		if err != nil {
			http.Error(writer, err.Error(), errCode)
			return
		}
		result, err, errCode := control.ReadDevice(id)
		if err != nil {
			http.Error(writer, err.Error(), errCode)
			return
		}
		//like the device-repository: with owner_id the device is looked up by owner and local id
		//and the caller needs read access; without it nothing is checked (behavior of the other tests)
		if ownerId := request.URL.Query().Get("owner_id"); ownerId != "" {
			token, err := jwt.GetParsedToken(request)
			if err != nil {
				http.Error(writer, err.Error(), http.StatusUnauthorized)
				return
			}
			err, errCode = control.CheckLocalDeviceRead(token.GetUserId(), ownerId, params.ByName("id"), result)
			if err != nil {
				http.Error(writer, err.Error(), errCode)
				return
			}
		}
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		err = json.NewEncoder(writer).Encode(result)
		if err != nil {
			log.Println("ERROR: unable to encode response", err)
		}
		return
	})

	//test setup: grants a user read access to a device of another owner, used by GET /local-devices/:id?owner_id=
	router.PUT("/mock/local-device-access", func(writer http.ResponseWriter, request *http.Request, params httprouter.Params) {
		grant := LocalDeviceAccessGrant{}
		err := json.NewDecoder(request.Body).Decode(&grant)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		control.GrantLocalDeviceAccess(grant)
		writer.WriteHeader(http.StatusOK)
	})

	router.POST(resource, func(writer http.ResponseWriter, request *http.Request, params httprouter.Params) {
		device := model.Device{}
		err := json.NewDecoder(request.Body).Decode(&device)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		if device.OwnerId == "" {
			token, err := jwt.GetParsedToken(request)
			if err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
			device.OwnerId = token.GetUserId()
		}
		result, err, errCode := control.PublishDeviceCreate(device)
		if err != nil {
			http.Error(writer, err.Error(), errCode)
			return
		}
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		err = json.NewEncoder(writer).Encode(result)
		if err != nil {
			log.Println("ERROR: unable to encode response", err)
		}
		return
	})

	router.PUT(resource+"/:id", func(writer http.ResponseWriter, request *http.Request, params httprouter.Params) {
		id := params.ByName("id")
		id, err, errCode := control.DeviceLocalIdToId(id)
		if err != nil {
			http.Error(writer, err.Error(), errCode)
			return
		}
		device := model.Device{}
		err = json.NewDecoder(request.Body).Decode(&device)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		if device.OwnerId == "" {
			token, err := jwt.GetParsedToken(request)
			if err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
			device.OwnerId = token.GetUserId()
		}
		result, err, errCode := control.PublishDeviceUpdate(id, device)
		if err != nil {
			http.Error(writer, err.Error(), errCode)
			return
		}
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		err = json.NewEncoder(writer).Encode(result)
		if err != nil {
			log.Println("ERROR: unable to encode response", err)
		}
		return
	})

	router.DELETE(resource+"/:id", func(writer http.ResponseWriter, request *http.Request, params httprouter.Params) {
		id := params.ByName("id")
		id, err, errCode := control.DeviceLocalIdToId(id)
		if err != nil {
			http.Error(writer, err.Error(), errCode)
			return
		}
		err, errCode = control.PublishDeviceDelete(id)
		if err != nil {
			http.Error(writer, err.Error(), errCode)
			return
		}
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		err = json.NewEncoder(writer).Encode(true)
		if err != nil {
			log.Println("ERROR: unable to encode response", err)
		}
		return
	})
}
