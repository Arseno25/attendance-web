package attendance

import (
	"attendance.local/api/internal/platform/aiclient"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestRecognitionApprovalRequiresLiveStableEvidenceAndModel(t *testing.T) {
	payload := map[string]any{"decision": "accepted", "recognition_enabled": true, "model_sha256": strings.Repeat("a", 64), "faces": []any{map[string]any{"student_id": "synthetic"}}, "liveness_passed": true, "is_confirmed": true}
	check := func() bool {
		raw, _ := json.Marshal(payload)
		var result aiclient.Recognition
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		return recognitionApproved(result)
	}
	if !check() {
		t.Fatal("live stable recognition incorrectly requires an offline report")
	}
	for _, field := range []string{"recognition_enabled", "liveness_passed", "is_confirmed"} {
		payload[field] = false
		if check() {
			t.Fatal("missing runtime evidence accepted", field)
		}
		payload[field] = true
	}
	payload["model_sha256"] = strings.Repeat("z", 64)
	if check() {
		t.Fatal("invalid model hash accepted")
	}
	payload["model_sha256"] = strings.Repeat("a", 64)
	payload["decision"] = "unknown"
	if check() {
		t.Fatal("unknown identity accepted")
	}

	for _, box := range []aiclient.Box{{X: math.NaN(), Width: 0.2, Height: 0.2}, {X: 0.8, Width: 0.3, Height: 0.2}, {Width: 0, Height: 0.2}, {X: -0.1, Width: 0.2, Height: 0.2}} {
		if validBox(box) {
			t.Fatal("invalid box accepted")
		}
	}
	if !validBox(aiclient.Box{X: 0.1, Y: 0.2, Width: 0.3, Height: 0.4}) {
		t.Fatal("normalized box rejected")
	}
}
