// Copyright Splunk, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build integration

package tests

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/signalfx/splunk-otel-collector/tests/testutils"
)

func TestDefaultContainerConfigRequiresEnvVars(t *testing.T) {
	image := (&testutils.Testcase{T: t}).SkipIfNotContainer()

	tests := []struct {
		name    string
		env     map[string]string
		missing string
	}{
		{"missing realm", map[string]string{
			"SPLUNK_REALM":        "",
			"SPLUNK_ACCESS_TOKEN": "some_token",
		}, "SPLUNK_REALM"},
		{"missing token", map[string]string{
			"SPLUNK_REALM":        "some_realm",
			"SPLUNK_ACCESS_TOKEN": "",
		}, "SPLUNK_ACCESS_TOKEN"},
	}
	for _, testcase := range tests {
		t.Run(testcase.name, func(tt *testing.T) {
			logCore, logs := observer.New(zap.DebugLevel)
			logger := zap.New(logCore)

			collector, err := testutils.NewCollectorContainer().WithImage(image).WithEnv(testcase.env).WithLogger(logger).WillFail(true).Build()
			require.NoError(t, err)
			require.NotNil(t, collector)
			defer collector.Shutdown()
			require.NoError(t, collector.Start())

			expectedError := fmt.Sprintf("ERROR: Missing required environment variable %s with default config path /etc/otel/collector/gateway_config.yaml", testcase.missing)
			require.Eventually(t, func() bool {
				for _, log := range logs.All() {
					if strings.Contains(log.Message, expectedError) {
						return true
					}
				}
				return false
			}, 30*time.Second, time.Second)
		})
	}
}

func TestSpecifiedContainerConfigDefaultsToCmdLineArgIfEnvVarConflict(t *testing.T) {
	tc := testutils.NewTestcase(t)
	defer tc.PrintLogsOnFailure()
	defer tc.ShutdownOTLPReceiverSink()

	tc.SkipIfNotContainer()

	_, shutdown := tc.SplunkOtelCollector(
		"hostmetrics_cpu.yaml",
		func(collector testutils.Collector) testutils.Collector {
			return collector.WithArgs("--config", "/etc/config.yaml")
		},
		func(collector testutils.Collector) testutils.Collector {
			return collector.WithEnv(
				map[string]string{
					"SPLUNK_CONFIG": "/not/a/real/path",
				},
			)
		},
	)

	defer shutdown()

	require.Eventually(t, func() bool {
		for _, log := range tc.ObservedLogs.All() {
			if strings.Contains(
				log.Message,
				`Both environment variable SPLUNK_CONFIG and flag '--config' were specified. `+
					`Using the flag values and ignoring the environment variable value `+
					`/not/a/real/path in this session`,
			) {
				return true
			}
		}
		return false
	}, 20*time.Second, time.Second)

	// confirm successful service functionality
	expectedResourceMetrics := tc.ResourceMetrics("cpu.yaml")
	require.NoError(t, tc.OTLPReceiverSink.AssertAllMetricsReceived(t, *expectedResourceMetrics, 30*time.Second))
}

func TestConfigYamlEnvVar(t *testing.T) {
	tc := testutils.NewTestcase(t)
	defer tc.PrintLogsOnFailure()
	defer tc.ShutdownOTLPReceiverSink()

	tc.SkipIfNotContainer()

	_, shutdown := tc.SplunkOtelCollector(
		"", func(collector testutils.Collector) testutils.Collector {
			return collector.WithEnv(
				map[string]string{
					"SPLUNK_CONFIG": "",
					"SPLUNK_CONFIG_YAML": `receivers:
  hostmetrics:
    collection_interval: 1s
    scrapers:
      cpu:

exporters:
  otlp:
    endpoint: "${OTLP_ENDPOINT}"
    insecure: true

service:
  pipelines:
    metrics:
      receivers: [hostmetrics]
      exporters: [otlp]
`},
			)
		},
	)

	defer shutdown()

	require.Eventually(t, func() bool {
		for _, log := range tc.ObservedLogs.All() {
			if strings.Contains(
				log.Message,
				`Using environment variable SPLUNK_CONFIG_YAML for configuration`,
			) {
				return true
			}
		}
		return false
	}, 20*time.Second, time.Second)

	// confirm successful service functionality
	expectedResourceMetrics := tc.ResourceMetrics("cpu.yaml")
	require.NoError(t, tc.OTLPReceiverSink.AssertAllMetricsReceived(t, *expectedResourceMetrics, 30*time.Second))
}
