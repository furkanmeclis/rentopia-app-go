package queue

import (
	"errors"
	"testing"

	"github.com/hibiken/asynq"
)

func TestPayloadIDs(t *testing.T) {
	got := payloadIDs([]byte(`{"organization_id":7,"job_id":42,"spec":"vehicles","note":"x","zero_id":0,"nested":{"user_id":3}}`))
	if len(got) != 2 || got["organization_id"] != 7 || got["job_id"] != 42 {
		t.Fatalf("ids: %v", got)
	}
	if payloadIDs([]byte(`[1,2]`)) != nil || payloadIDs(nil) != nil || payloadIDs([]byte(`{"a":"b"}`)) != nil {
		t.Fatal("non-object or id-less payloads give nil")
	}
}

func TestDescribeFailureFinal(t *testing.T) {
	task := asynq.NewTask("app:test", []byte(`{"export_job_id":9}`))
	f := describeFailure(t.Context(), task, errors.Join(errors.New("boom"), asynq.SkipRetry))
	if !f.Final || f.Type != "app:test" || f.IDs["export_job_id"] != 9 {
		t.Fatalf("failure %+v", f)
	}
}
