/*
 * Copyright (c) 2023 InfAI (CC SES)
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

package metrics

import (
	"errors"
	"github.com/SENERGY-Platform/platform-connector-lib/statistics"
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
)

type Metrics struct {
	ClientErrorMessages prometheus.Counter
	httphandler         http.Handler
}

// NewMetrics creates metrics will be exposed /metrics by github.com/SENERGY-Platform/platform-connector-lib/statistics/statistics.go
func NewMetrics() (*Metrics, error) {
	statistics.Init()
	m := &Metrics{
		ClientErrorMessages: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "senergy_connector_client_error_messages",
			Help: "count of error messages received since startup",
		}),
	}
	err := prometheus.DefaultRegisterer.Register(m.ClientErrorMessages)
	var alreadyReg prometheus.AlreadyRegisteredError
	if errors.As(err, &alreadyReg) {
		var ok bool
		m.ClientErrorMessages, ok = alreadyReg.ExistingCollector.(prometheus.Counter)
		if !ok {
			return m, err
		}
		err = nil
	}
	if err != nil {
		return m, err
	}
	return m, nil
}
