package attendance

import "testing"

func TestCSVTextPreservesOrdinaryTextAndBlocksFormulaPrefixes(t *testing.T) {
	for _, v := range []string{"=SUM(1,2)", "+cmd", "-cmd", "@cmd", "  =formula", "\ttext", "\rtext", "\ntext", "＝cmd", "＋cmd", "－cmd", "＠cmd"} {
		if got := csvText(v); got != "'"+v {
			t.Errorf("unsafe prefix not escaped: %q -> %q", v, got)
		}
	}
	for _, v := range []string{"001", "Nama siswa", "Nama, dengan koma", "text \"quoted\"", "", "teks\nbaris baru"} {
		if csvText(v) != v {
			t.Errorf("ordinary value changed: %q", v)
		}
	}
}
