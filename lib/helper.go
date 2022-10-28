/*
 * Copyright 2021 InfAI (CC SES)
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

package lib

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"log"
	"strings"
)

func StringToList(str string) []string {
	temp := strings.Split(str, ",")
	result := []string{}
	for _, e := range temp {
		trimmed := strings.TrimSpace(e)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func CreateTLSConfig(ClientCertificatePath string, PrivateKeyPath string, RootCACertificatePath string) (tlsConfig *tls.Config, err error) {
	// Load Client/Server Certificate
	var cer tls.Certificate
	cer, err = tls.LoadX509KeyPair(ClientCertificatePath, PrivateKeyPath)
	if err != nil {
		log.Println("Error on TLS: cant read client certificate", err)
		return nil, err
	}

	// Load CA cert
	caCert, err := ioutil.ReadFile(RootCACertificatePath)
	if err != nil {
		log.Println("Error on TLS: cant read CA certificate", err)
		return nil, err
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	tlsConfig = &tls.Config{
		Certificates: []tls.Certificate{cer},
		RootCAs:      caCertPool,

		// This circumvents the need for matching request hostname and server certficate CN field
		InsecureSkipVerify: true,
	}
	return
}
