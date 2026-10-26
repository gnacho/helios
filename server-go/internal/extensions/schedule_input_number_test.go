package extensions

import (
	"testing"
	"time"
)

type scheduleCall struct {
	domain  string
	service string
	data    map[string]any
}

type recordingScheduleHA struct {
	*fakeHA
	calls []scheduleCall
}

func (r *recordingScheduleHA) CallService(domain, service string, data map[string]any) error {
	r.calls = append(r.calls, scheduleCall{domain: domain, service: service, data: data})
	return nil
}

// Regression (issue 175): production uses input_number helpers for the
// portable scheduling dps, and ApplyTimed rejected them with ErrChargerNoAmp.
func TestApplyTimedInputNumberHelpers(t *testing.T) {
	f := &recordingScheduleHA{fakeHA: newFake()}
	ch := Charger{
		Key:        "c2",
		DelayID:    "input_number.feyree_delay_h",
		DurationID: "input_number.feyree_duration_h",
	}
	at := time.Now().Add(2 * time.Hour).Format("15:04")
	if err := ApplyTimed(f, ch, at, 1); err != nil {
		t.Fatalf("ApplyTimed: %v", err)
	}
	if len(f.calls) != 2 {
		t.Fatalf("calls = %d, want 2 (%v)", len(f.calls), f.calls)
	}
	for _, c := range f.calls {
		if c.domain != "input_number" || c.service != "set_value" {
			t.Fatalf("call = %s.%s, want input_number.set_value", c.domain, c.service)
		}
	}
	if got := f.calls[0].data["entity_id"]; got != ch.DelayID {
		t.Fatalf("first entity = %v, want %s", got, ch.DelayID)
	}
	if got := f.calls[1].data["entity_id"]; got != ch.DurationID {
		t.Fatalf("second entity = %v, want %s", got, ch.DurationID)
	}
	if got := f.calls[1].data["value"]; got != 1.0 {
		t.Fatalf("duration value = %v, want 1", got)
	}
	delay, ok := f.calls[0].data["value"].(float64)
	if !ok || delay < 0 || delay > 24 {
		t.Fatalf("delay value = %v, want hours in [0,24]", f.calls[0].data["value"])
	}
}

// The delay/duration actions share the same validation and must write with the
// real entity domain instead of number.set_value.
func TestApplyActionDelayInputNumber(t *testing.T) {
	f := &recordingScheduleHA{fakeHA: newFake()}
	ch := Charger{Key: "c2", DelayID: "input_number.feyree_delay_h"}
	v := 1.5
	got, err := ApplyAction(f, ch, "delay", &v)
	if err != nil {
		t.Fatalf("ApplyAction: %v", err)
	}
	if got != 1.5 {
		t.Fatalf("value = %v, want 1.5", got)
	}
	if len(f.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(f.calls))
	}
	c := f.calls[0]
	if c.domain != "input_number" || c.service != "set_value" {
		t.Fatalf("call = %s.%s, want input_number.set_value", c.domain, c.service)
	}
	if c.data["entity_id"] != ch.DelayID || c.data["value"] != 1.5 {
		t.Fatalf("data = %v", c.data)
	}
}
