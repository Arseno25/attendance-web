package attendance

import (
	"attendance.local/api/internal/platform/aiclient"
	"math"
	"strings"
	"testing"
)

func TestRecognitionApprovalRequiresEveryEvaluatedGateAndModelBinding(t *testing.T) {
	result := aiclient.Recognition{Decision: "accepted", RecognitionEnabled: true, ModelSHA: strings.Repeat("a", 64), Faces: []aiclient.Face{{StudentID: "synthetic"}}}
	if recognitionApproved(result) {
		t.Fatal("an accepted flag bypassed evaluation")
	}
	result.Evaluation = map[string]any{"model_sha256": result.ModelSHA, "report_sha256": strings.Repeat("b", 64), "automatic_attendance_approved": true, "holdout_verified": true, "liveness_verified": true, "stability_verified": true, "threshold_margin_verified": true}
	if !recognitionApproved(result) {
		t.Fatal("complete synthetic evidence contract rejected")
	}
	for _, field := range []string{"automatic_attendance_approved", "holdout_verified", "liveness_verified", "stability_verified", "threshold_margin_verified"} {
		result.Evaluation[field] = false
		if recognitionApproved(result) {
			t.Fatal("missing gate accepted", field)
		}
		result.Evaluation[field] = true
	}
	result.Evaluation["model_sha256"] = strings.Repeat("c", 64)
	if recognitionApproved(result) {
		t.Fatal("different evaluated model accepted")
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
