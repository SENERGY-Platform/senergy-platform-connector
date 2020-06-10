package iot

import (
	"encoding/json"
	"github.com/SENERGY-Platform/platform-connector-lib/model"
	"github.com/julienschmidt/httprouter"
	"log"
	"net/http"
)

func CharacteristicsEndpoints(control *Controller, router *httprouter.Router) {
	resource := "/characteristics"

	router.GET(resource+"/:id", func(writer http.ResponseWriter, request *http.Request, params httprouter.Params) {
		id := params.ByName("id")
		result, err, errCode := control.ReadCharacteristic(id)
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

	router.POST(resource, func(writer http.ResponseWriter, request *http.Request, params httprouter.Params) {
		characteristic := model.Characteristic{}
		err := json.NewDecoder(request.Body).Decode(&characteristic)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		result, err, errCode := control.PublishCharacteristicCreate(characteristic)
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
		characteristic := model.Characteristic{}
		err := json.NewDecoder(request.Body).Decode(&characteristic)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		result, err, errCode := control.PublishCharacteristicUpdate(id, characteristic)
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
		err, errCode := control.PublishCharacteristicDelete(id)
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
