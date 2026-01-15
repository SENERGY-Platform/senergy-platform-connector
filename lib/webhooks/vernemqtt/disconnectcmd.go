/*
 * Copyright 2025 InfAI (CC SES)
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package vernemqtt

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"time"

	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
)

// disconnectCommand godoc
// @Summary      disconnected command webhook
// @Description  disconnects mqtt clients of given user; all responses are with code=200, differences in swagger doc are because of technical incompatibilities of the documentation format
// @Accept       json
// @Produce      json
// @Param        message body DisconnectCommand true "disconnect command info"
// @Success      200 {object}  OkResponse
// @Failure      400 {object}  ErrorResponse
// @Router       /disconnect-command [POST]
func disconnectCommand(writer http.ResponseWriter, request *http.Request, config configuration.Config, logger *slog.Logger) {
	defer func() {
		if p := recover(); p != nil {
			if config.Debug {
				debug.PrintStack()
			}
			sendError(writer, fmt.Sprint(p), true)
			return
		}
	}()
	msg := DisconnectCommand{}
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		sendError(writer, err.Error(), true)
		return
	}

	if config.VmqAdminApiUrl == "" || config.VmqAdminApiUrl == "-" {
		sendError(writer, "missing VmqAdminApiUrl config", true)
		return
	}

	if config.DisconnectCommandDelay != "" && config.DisconnectCommandDelay != "-" {
		duration, err := time.ParseDuration(config.DisconnectCommandDelay)
		if err != nil {
			logger.Error("disconnect-cmd error: unable to parse config.DisconnectCommandDelay", "action", "disconnect-cmd", "error", err)
			sendError(writer, err.Error(), false)
			return
		}
		time.AfterFunc(duration, func() {
			err = execDisconnectCommand(config, msg, logger)
			if err != nil {
				logger.Error("disconnect-cmd error", "action", "disconnect-cmd", "username", msg.Username, "error", err)
			}
		})
	} else {
		err = execDisconnectCommand(config, msg, logger)
		if err != nil {
			logger.Error("disconnect-cmd error", "action", "disconnect-cmd", "username", msg.Username, "error", err)
		}
	}

	_, err = fmt.Fprint(writer, `{"result": "ok"}`)
	if err != nil {
		config.GetLogger().Debug("ERROR: InitWebhooks::disconnectCommand unable to fprint", "error", err)
	}
}

func execDisconnectCommand(config configuration.Config, cmd DisconnectCommand, logger *slog.Logger) (err error) {
	logger.Info("disconnect-cmd", "action", "disconnect-cmd", "username", cmd.Username)
	clientIds, err := listSessionsClientIdsOfUser(config.VmqAdminApiUrl, cmd.Username)
	if err != nil {
		return err
	}
	for _, clientId := range clientIds {
		err = disconnectClient(config.VmqAdminApiUrl, clientId)
		if err != nil {
			return err
		}
	}
	return nil
}

func listSessionsClientIdsOfUser(apiUrl string, user string) (result []string, err error) {
	// vmq-admin session show --user=<user>

	path := "/api/v1/session/show?--user=" + url.QueryEscape(user)
	req, err := http.NewRequest("GET", apiUrl+path, nil)
	if err != nil {
		return nil, fmt.Errorf("ubable to get user sessions: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ubable to get user sessions: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		buf, _ := io.ReadAll(resp.Body)
		err = fmt.Errorf("ubable to get user sessions: status-code: %v response: %v", resp.StatusCode, string(buf))
		return nil, err
	}
	temp := SubscriptionWrapper{}
	err = json.NewDecoder(resp.Body).Decode(&temp)
	if err != nil {
		return nil, fmt.Errorf("ubable to decode user sessions: %w", err)
	}
	for _, sub := range temp.Table {
		result = append(result, sub.ClientId)
	}
	return result, nil
}

func disconnectClient(apiUrl string, clientId string) (err error) {
	// vmq-admin session disconnect client-id=<id> --cleanup
	endpoint := fmt.Sprintf("%s/api/v1/session/disconnect?client-id=%s&--cleanup", apiUrl, url.QueryEscape(clientId))
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return fmt.Errorf("ubable to disconnect client session: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("ubable to disconnect client session: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		buf, _ := io.ReadAll(resp.Body)
		err = fmt.Errorf("ubable to disconnect client session: status-code: %v response: %v", resp.StatusCode, string(buf))
		return err
	}
	return nil
}

type SubscriptionWrapper struct {
	Table []Subscription `json:"table"`
}

type Subscription struct {
	ClientId string `json:"client_id"`
	User     string `json:"user"`
	Topic    string `json:"topic"`
	IsOnline bool   `json:"is_online"`
}
