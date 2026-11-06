package storage

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"api-test-kits/internal/domain"
)

func TestSocketRequestRoundTripAndDuplicate(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	c, err := s.SaveCollection(ctx, domain.Collection{Name: "C"})
	if err != nil {
		t.Fatal(err)
	}
	in := domain.Request{CollectionID: c.ID, Name: "events", Kind: domain.KindSocketIO, URLTemplate: "{{base}}/admin",
		Socket: &domain.SocketConfig{
			AuthPayload: `{"token":"{{token}}"}`,
			Messages:    []domain.SocketMessage{{Name: "hello", Event: "echo", Body: `{"a":1}`, BodyType: domain.SocketBodyJSON}},
			Steps: []domain.SocketStep{
				{Kind: domain.StepSend, Enabled: true, Message: domain.SocketMessage{Event: "echo", Body: "x", Ack: true}},
				{Kind: domain.StepExpect, Enabled: true, Event: "echo", ExpectAck: true, Assertions: []domain.Assertion{
					{Kind: domain.AssertJSONPath, Path: "a", Operator: domain.OpEq, Expected: json.RawMessage("1"), Enabled: true}}},
			},
		}}
	saved, err := s.SaveRequest(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetRequest(ctx, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != domain.KindSocketIO || got.Method != "GET" || got.Socket == nil {
		t.Fatalf("got %+v", got)
	}
	sc := got.Socket
	if sc.Path != domain.DefaultSocketIOPath || sc.AuthPayload != in.Socket.AuthPayload || len(sc.Messages) != 1 ||
		len(sc.Steps) != 2 || !sc.Steps[1].ExpectAck || sc.Steps[1].TimeoutMs != domain.DefaultExpectTimeout ||
		string(sc.Steps[1].Assertions[0].Expected) != "1" || !sc.Steps[0].Message.Ack {
		t.Fatalf("socket settings %+v", sc)
	}
	dup, err := s.DuplicateRequest(ctx, saved.ID)
	if err != nil || dup.Kind != domain.KindSocketIO || len(dup.Socket.Steps) != 2 {
		t.Fatalf("duplicate %+v %v", dup, err)
	}

	// Switching back to HTTP drops the socket settings.
	got.Kind = domain.KindHTTP
	if got, err = s.SaveRequest(ctx, got); err != nil || got.Socket != nil {
		t.Fatalf("http again: %+v %v", got.Socket, err)
	}
}

func TestSocketTranscriptIsStoredWithinTheBodyLimit(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	frames := []domain.SocketFrame{
		{Direction: domain.FrameSystem, Data: "connected"},
		{Direction: domain.FrameReceived, Data: strings.Repeat("a", 30)},
		{Direction: domain.FrameReceived, Data: "dropped"},
	}
	now := time.Now().UTC()
	run := domain.Run{ID: "r1", StartedAt: now, FinishedAt: &now, Status: domain.StatusPassed, Results: []domain.RunResult{{
		ID: "res1", Status: domain.StatusPassed, AssertionResults: []domain.AssertionResult{}, CreatedAt: now,
		Socket: &domain.SocketTranscript{Kind: domain.KindWebSocket, Frames: frames, CloseCode: 1000},
	}}}
	if err := s.SaveRun(ctx, run, 20, 0); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetRun(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	tr := got.Results[0].Socket
	if tr == nil || tr.CloseCode != 1000 || len(tr.Frames) != 2 || tr.Dropped != 1 {
		t.Fatalf("transcript %+v", tr)
	}
	if f := tr.Frames[1]; len(f.Data) != 11 || !f.Truncated {
		t.Fatalf("second frame %+v", f)
	}
}
