package holidays

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestYearContract(t *testing.T) {
	valid := `{"meta":{"year":2026,"count":2},"data":[{"date":"2026-01-01","name":"Tahun Baru","type":"national_holiday"},{"date":"2026-03-20","name":"Cuti bersama","type":"joint_leave"}]}`
	for _, tc := range []struct {
		name, body string
		status     int
		ok         bool
	}{
		{"national_and_joint_leave", valid, 200, true},
		{"empty_year", `{"meta":{"year":2026,"count":0},"data":[]}`, 200, false},
		{"wrong_year", strings.ReplaceAll(valid, "2026-01-01", "2027-01-01"), 200, false},
		{"invalid_date", strings.ReplaceAll(valid, "2026-01-01", "2026-02-30"), 200, false},
		{"unknown_kind", strings.ReplaceAll(valid, "joint_leave", "unknown"), 200, false},
		{"count_mismatch", strings.ReplaceAll(valid, `"count":2`, `"count":3`), 200, false},
		{"trailing_data", valid + `{}`, 200, false},
		{"oversized_valid_json", strings.Replace(valid, `"meta"`, `"padding":"`+strings.Repeat("x", 512<<10)+`","meta"`, 1), 200, false},
		{"unavailable", `{}`, 503, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/holidays/2026" || r.URL.RawQuery != "" {
					t.Errorf("unexpected URL: %s", r.URL)
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			days, err := New(server.URL).Year(context.Background(), 2026)
			if (err == nil) != tc.ok {
				t.Fatalf("days=%v err=%v", days, err)
			}
			if tc.ok && (len(days) != 2 || days[1].Type != "joint_leave") {
				t.Fatal(days)
			}
		})
	}
}
func TestInvalidYearNeverCallsProvider(t *testing.T) {
	for _, year := range []int{0, 1899, 2201} {
		if _, err := New("http://invalid.local").Year(context.Background(), year); err == nil {
			t.Fatal("accepted invalid year")
		}
	}
}

func TestProviderRedirectIsNotFollowed(t *testing.T) {
	targetCalled := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalled = true
		fmt.Fprint(w, `{"meta":{"year":2026,"count":1},"data":[{"date":"2026-01-01","name":"New Year","type":"national_holiday"}]}`)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer source.Close()
	if _, err := New(source.URL).Year(context.Background(), 2026); err == nil || targetCalled {
		t.Fatal("provider redirected to another endpoint")
	}
}
