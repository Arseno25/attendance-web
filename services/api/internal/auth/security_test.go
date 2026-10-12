package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPasswordHashIsSaltedAndRejectsInvalidParameters(t *testing.T) {
	password := strings.Repeat("a", 16)
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if hash == second || !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatal("salted Argon2id hash required")
	}
	if !VerifyPassword(password, hash) || VerifyPassword("wrong", hash) {
		t.Fatal("password verification failed")
	}
	for _, invalid := range []string{"", "junk", "$argon2id$v=19$m=999999999,t=3,p=2$bad$bad", strings.Replace(hash, "v=19", "v=20", 1)} {
		if VerifyPassword(password, invalid) {
			t.Fatal("unsafe hash accepted")
		}
	}
	for _, invalid := range []string{"short", strings.Repeat("a", 129)} {
		if _, err := HashPassword(invalid); err == nil {
			t.Fatal("password size not enforced")
		}
	}
}

func TestPolicyEnforcesSchoolClassAndSecurityRole(t *testing.T) {
	admin := User{SchoolID: "school-a", Role: "super_admin", Active: true}
	if !admin.Can("users.write", "school-a", "") {
		t.Fatal("super admin denied own school")
	}
	if admin.Can("users.write", "school-b", "") {
		t.Fatal("cross-school access allowed")
	}
	teacher := User{SchoolID: "school-a", Role: "teacher", Active: true, ClassIDs: []string{"class-a"}, Grants: []string{"users.write"}}
	if !teacher.Can("attendance.read", "school-a", "class-a") {
		t.Fatal("assigned class denied")
	}
	for _, classID := range []string{"", "class-b"} {
		if teacher.Can("attendance.read", "school-a", classID) {
			t.Fatal("unassigned scope allowed")
		}
	}
	if teacher.Can("users.write", "school-a", "") || teacher.Can("audit.read", "school-a", "") {
		t.Fatal("security role bypass")
	}
	teacher.Active = false
	if teacher.Can("attendance.read", "school-a", "class-a") {
		t.Fatal("inactive account authorized")
	}
}

func TestLimiterDeniesBurstAndResetsAfterWindow(t *testing.T) {
	var l limiter
	now := time.Now()
	if !l.allow("ip", 2, now) || !l.allow("ip", 2, now) || l.allow("ip", 2, now) {
		t.Fatal("burst quota ignored")
	}
	if !l.allow("ip", 2, now.Add(16*time.Minute)) {
		t.Fatal("expired quota did not reset")
	}
}

func TestOperatorCannotGainRestrictedAccessFromGrants(t *testing.T) {
	u := User{SchoolID: "school-a", Role: "operator", Active: true, Grants: []string{"reports.export", "biometrics.revoke", "consents.manage", "attendance.approve"}}
	for _, permission := range u.Grants {
		if u.Can(permission, "school-a", "") {
			t.Fatalf("operator gained %s", permission)
		}
	}
}

func TestHTTPSCookieAndDeletion(t *testing.T) {
	s, err := New(nil, "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA", "https://school.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if s.schoolID != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" {
		t.Fatal("school UUID not canonical")
	}
	for _, remove := range []bool{false, true} {
		w := httptest.NewRecorder()
		s.setCookie(w, "opaque", remove)
		c := w.Result().Cookies()[0]
		if c.Name != "__Host-attendance_session" || !c.Secure || !c.HttpOnly || c.Domain != "" || c.Path != "/" || c.SameSite != http.SameSiteLaxMode {
			t.Fatal("cookie protection missing")
		}
		if (!remove && c.MaxAge != 28800) || (remove && c.MaxAge != -1) {
			t.Fatal("cookie lifetime wrong")
		}
	}
}
