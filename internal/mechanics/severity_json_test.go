package mechanics_test

import (
	"encoding/json"
	"testing"

	"github.com/use-assay/assay/internal/mechanics"
)

func TestSeverityJSONRoundTrip(t *testing.T) {
	values := []mechanics.Severity{
		mechanics.Clear,
		mechanics.Low,
		mechanics.Medium,
		mechanics.High,
		mechanics.Critical,
		mechanics.Unevaluated,
	}
	for _, want := range values {
		t.Run(want.String(), func(t *testing.T) {
			encoded, err := json.Marshal(want)
			if err != nil {
				t.Fatalf("marshal %s: %v", want, err)
			}
			var got mechanics.Severity
			if err := json.Unmarshal(encoded, &got); err != nil {
				t.Fatalf("unmarshal %s: %v", encoded, err)
			}
			if got != want {
				t.Fatalf("round trip = %v, want %v", got, want)
			}
		})
	}
}

func TestSeverityJSONRejectsUnknownName(t *testing.T) {
	var got mechanics.Severity
	if err := json.Unmarshal([]byte(`"unrecognized"`), &got); err == nil {
		t.Fatal("unknown severity was accepted")
	}
}
