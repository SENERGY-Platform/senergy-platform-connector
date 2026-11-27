/*
 * Copyright 2026 InfAI (CC SES)
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
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
)

// tokenSessionClientIdPrefix marks the sessions of token logins: the login webhook renames them to
// "web:<exp unix seconds>:<original client id>". The prefix is exclusive to token sessions; certificate and
// password logins with such a client id are refused, so publish and subscribe can rely on it.
const tokenSessionClientIdPrefix = "web:"

// tokenSessionSweepInterval is a variable for the tests only
var tokenSessionSweepInterval = 60 * time.Second

func isTokenSessionClientId(clientId string) bool {
	return strings.HasPrefix(clientId, tokenSessionClientIdPrefix)
}

func tokenSessionClientId(exp time.Time, originalClientId string) string {
	return tokenSessionClientIdPrefix + strconv.FormatInt(exp.Unix(), 10) + ":" + originalClientId
}

// tokenSessionExpiry returns the expiry encoded in the client id of a token session
func tokenSessionExpiry(clientId string) (time.Time, error) {
	if !isTokenSessionClientId(clientId) {
		return time.Time{}, errors.New("not a token session client id")
	}
	parts := strings.SplitN(strings.TrimPrefix(clientId, tokenSessionClientIdPrefix), ":", 2)
	if len(parts) != 2 {
		return time.Time{}, errors.New("token session client id without expiry")
	}
	exp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("token session client id with invalid expiry: %w", err)
	}
	return time.Unix(exp, 0), nil
}

// accessTokenExpiry reads the exp claim of a Keycloak access token. It does not verify anything: the token has to be
// validated before. The expiry is read here because jwt.ParseWithValidation of service-commons checks the signature
// but not exp (its Token.Valid only demands a subject). The typ claim has to be "Bearer", which Keycloak sets on
// access tokens only, so an ID token of the same realm cannot log in.
func accessTokenExpiry(token string) (time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, errors.New("token is not a jwt")
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return time.Time{}, fmt.Errorf("unable to decode token payload: %w", err)
	}
	claims := struct {
		Exp *float64 `json:"exp"`
		Typ string   `json:"typ"`
	}{}
	err = json.Unmarshal(payload, &claims)
	if err != nil {
		return time.Time{}, fmt.Errorf("unable to parse token payload: %w", err)
	}
	if claims.Typ != "Bearer" {
		return time.Time{}, errors.New("token is not an access token")
	}
	if claims.Exp == nil || *claims.Exp <= 0 {
		return time.Time{}, errors.New("token has no expiry")
	}
	return time.Unix(int64(*claims.Exp), 0), nil
}

// StartTokenSessionSweeper disconnects the sessions of token logins whose token has expired.
// VerneMQ checks the password only at login, so a session would otherwise outlive its token.
// It does nothing if token logins are disabled, and needs VmqAdminApiUrl to find and disconnect the sessions.
func StartTokenSessionSweeper(ctx context.Context, config configuration.Config) {
	if config.TokenLoginMountpoint == "" {
		return
	}
	logger := config.GetLogger().With("snrgy-log-type", "connector-webhook")
	if config.VmqAdminApiUrl == "" || config.VmqAdminApiUrl == "-" {
		logger.Warn("token sessions are not swept: missing VmqAdminApiUrl config", "action", "token-session-sweep")
		return
	}
	go func() {
		ticker := time.NewTicker(tokenSessionSweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				err := sweepTokenSessions(config.VmqAdminApiUrl, time.Now(), logger)
				if err != nil {
					logger.Error("token session sweep failed", "action", "token-session-sweep", "error", err)
				}
			}
		}
	}()
}

// sweepTokenSessions disconnects every token session whose expiry is not after now.
// A failing disconnect does not stop the sweep; the sessions that failed are retried by the next one.
func sweepTokenSessions(apiUrl string, now time.Time, logger *slog.Logger) error {
	clientIds, err := listAllSessionsClientIds(apiUrl)
	if err != nil {
		return err
	}
	var errs []error
	for _, clientId := range clientIds {
		if !isTokenSessionClientId(clientId) {
			continue
		}
		exp, err := tokenSessionExpiry(clientId)
		if err != nil {
			logger.Warn("unable to read expiry of token session", "action", "token-session-sweep", "clientId", clientId, "error", err)
			continue
		}
		if exp.After(now) {
			continue
		}
		logger.Info("disconnect expired token session", "action", "token-session-sweep", "clientId", clientId, "expired", exp.UTC().Format(time.RFC3339))
		err = disconnectClient(apiUrl, clientId)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
