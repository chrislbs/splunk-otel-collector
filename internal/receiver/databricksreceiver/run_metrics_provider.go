// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package databricksreceiver

import (
	"fmt"

	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/signalfx/splunk-otel-collector/internal/receiver/databricksreceiver/internal/metadata"
)

// runMetricsProvider provides metrics for job and task runs. It uses a
// runTracker to extract just the new runs returned from the API.
type runMetricsProvider struct {
	tracker  *runTracker
	dbClient databricksClientInterface
}

func newRunMetricsProvider(dbClient databricksClientInterface) runMetricsProvider {
	return runMetricsProvider{
		tracker:  newRunTracker(),
		dbClient: dbClient,
	}
}

func (p runMetricsProvider) addMultiJobRunMetrics(ms pmetric.MetricSlice, jobIDs []int) error {
	jobPts := initGauge(ms, metadata.M.DatabricksJobsRunDuration)
	taskPts := initGauge(ms, metadata.M.DatabricksTasksRunDuration)
	for _, jobID := range jobIDs {
		err := p.addSingleJobRunMetrics(jobPts, taskPts, jobID)
		if err != nil {
			return fmt.Errorf("runMetricsProvider.addMultiJobRunMetrics(): aborting: %w", err)
		}
	}
	return nil
}

func (p runMetricsProvider) addSingleJobRunMetrics(
	jobPts pmetric.NumberDataPointSlice,
	taskPts pmetric.NumberDataPointSlice,
	jobID int,
) error {
	startTime := p.tracker.getPrevStartTime(jobID)
	runs, err := p.dbClient.completedJobRuns(jobID, startTime)
	if err != nil {
		return fmt.Errorf("runMetricsProvider.addSingleJobRunMetrics(): %w", err)
	}
	newRuns := p.tracker.extractNewRuns(runs)
	for _, run := range newRuns {
		// consider skipping run.State.LifeCycleState == "TERMINATED" due to error
		if run.State.LifeCycleState == "SKIPPED" {
			continue
		}
		jobPt := jobPts.AppendEmpty()
		jobPt.SetIntValue(int64(run.ExecutionDuration))
		jobPt.Attributes().PutInt(metadata.Attributes.JobID, int64(jobID))
		for _, task := range run.Tasks {
			taskPt := taskPts.AppendEmpty()
			taskPt.SetIntValue(int64(task.ExecutionDuration))
			taskAttrs := taskPt.Attributes()
			taskAttrs.PutInt(metadata.Attributes.JobID, int64(jobID))
			taskAttrs.PutString(metadata.Attributes.TaskID, task.TaskKey)
		}
	}
	return nil
}
