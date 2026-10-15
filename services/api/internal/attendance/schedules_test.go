package attendance

import "testing"

func TestAutomaticCheckoutScheduleBounds(t *testing.T) {
	in := scheduleInput{Name: "Sekolah", Days: []int{1}, Start: "07:00", Cutoff: "07:30", Grace: 15, Close: "16:00"}
	for _, value := range []string{"", "15:00"} {
		in.Checkout = value
		if !validSchedule(in) {
			t.Fatal("valid schedule rejected", value)
		}
	}
	for _, value := range []string{"07:45", "07:30", "16:00", "24:00", "invalid"} {
		in.Checkout = value
		if validSchedule(in) {
			t.Fatal("invalid checkout accepted", value)
		}
	}
}
