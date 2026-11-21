// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// WorkflowRunRequest is the payload for POST /apps/{app_id}/workflow:run.
type WorkflowRunRequest struct {
	Inputs map[string]any `json:"inputs,omitempty"`
}

// workflowStreamEvent mirrors one SSE data payload of the workflow run
// stream. Dify emits `data: {json}` frames only (no `event:` lines); the
// event name lives inside the JSON.
type workflowStreamEvent struct {
	Event string          `json:"event"`
	Err   json.RawMessage `json:"err"`
	Data  struct {
		Status  string          `json:"status"`
		Outputs json.RawMessage `json:"outputs"`
		Error   string          `json:"error"`
	} `json:"data"`
}

// RunWorkflow executes a published workflow app via
// POST /openapi/v1/apps/{app_id}/workflow:run and returns the outputs of the
// workflow_finished stream event. The endpoint always answers 200 with a
// text/event-stream body; workflow failures surface inside the stream.
func (c *DifyClient) RunWorkflow(ctx context.Context, appID string, req WorkflowRunRequest) (json.RawMessage, error) {
	runURL := fmt.Sprintf("%s/apps/%s/workflow:run", c.BaseURL, url.PathEscape(appID))

	resp, err := c.doRequest(ctx, http.MethodPost, runURL, req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		data, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("workflow run failed with status %d (body unreadable: %v)", resp.StatusCode, readErr)
		}
		return nil, apiErrorFromBody(resp.StatusCode, data)
	}

	return parseWorkflowFinished(resp.Body)
}

// parseWorkflowFinished scans an SSE stream for the workflow_finished event
// and returns its outputs. Frames are `data: {json}` lines separated by
// blank lines; earlier progress events are ignored.
func parseWorkflowFinished(r io.Reader) (json.RawMessage, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var dataLines []string
	flush := func() (json.RawMessage, bool, error) {
		if len(dataLines) == 0 {
			return nil, false, nil
		}
		payload := strings.Join(dataLines, "\n")
		dataLines = nil

		var evt workflowStreamEvent
		if err := json.Unmarshal([]byte(payload), &evt); err != nil {
			return nil, false, fmt.Errorf("failed to unmarshal workflow stream event: %w", err)
		}
		switch evt.Event {
		case "workflow_finished":
			if evt.Data.Status != "succeeded" {
				return nil, true, fmt.Errorf("workflow run %s: %s", evt.Data.Status, evt.Data.Error)
			}
			return evt.Data.Outputs, true, nil
		case "workflow_failed":
			return nil, true, fmt.Errorf("workflow run failed: %s", evt.Data.Error)
		case "error":
			return nil, true, fmt.Errorf("workflow run error event: %s", string(evt.Err))
		}
		return nil, false, nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			outputs, done, err := flush()
			if err != nil || done {
				return outputs, err
			}
		case strings.HasPrefix(line, "data:"):
			dataLines = append(dataLines, strings.TrimPrefix(line, "data:"))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading workflow stream: %w", err)
	}
	outputs, done, err := flush()
	if err != nil {
		return nil, err
	}
	if !done {
		return nil, fmt.Errorf("workflow stream ended without workflow_finished event")
	}
	return outputs, nil
}
