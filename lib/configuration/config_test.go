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

package configuration

import (
	"testing"
)

func TestHandleEnvironmentVarsSetsKafkaTopicSettings(t *testing.T) {
	t.Setenv("KAFKA_REPLICATION_FACTOR", "2")
	t.Setenv("KAFKA_TOPIC_CONFIGS", `{"urn_infai_ses_service_":[{"ConfigName":"retention.ms","ConfigValue":"604800000"}]}`)

	config := &ConfigStruct{KafkaReplicationFactor: 1}
	HandleEnvironmentVars(config)

	if config.KafkaReplicationFactor != 2 {
		t.Fatalf("replication factor not applied: %d", config.KafkaReplicationFactor)
	}
	entries := config.KafkaTopicConfigs["urn_infai_ses_service_"]
	if len(entries) != 1 || entries[0].ConfigName != "retention.ms" || entries[0].ConfigValue != "604800000" {
		t.Fatalf("topic configs not applied: %#v", config.KafkaTopicConfigs)
	}
}
