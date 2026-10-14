package auth

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/argon2"
)

func TestPasswordHashIsSaltedAndRejectsInvalidParameters(t *testing.T) {
	password := "ExamplePassword1!"
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
			t.Fatal("password policy not enforced")
		}
	}
}

func TestPolicyEnforcesSchoolClassAndSecurityRole(t *testing.T) {
	admin := User{SchoolID: "school-a", Role: "super_admin", Active: true, Scope: ScopeSchool, PermissionsList: []string{"users.read", "users.write", "roles.manage"}}
	if !admin.Can("users.write", "school-a", "") {
		t.Fatal("super admin denied own school")
	}
	if admin.Can("users.write", "school-b", "") {
		t.Fatal("cross-school access allowed")
	}
	teacher := User{SchoolID: "school-a", Role: "teacher", Active: true, Scope: ScopeAssignedClasses, PermissionsList: []string{"attendance.read"}, ClassIDs: []string{"class-a"}}
	if !teacher.Can("attendance.read", "school-a", "class-a") {
		t.Fatal("assigned class denied")
	}
	for _, classID := range []string{"class-b"} {
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

func TestPolicyUsesDynamicRole(t *testing.T) {
	user := User{
		SchoolID: "school-a", Role: "custom-class-role", Active: true,
		Scope: ScopeAssignedClasses, PermissionsList: []string{"students.read", "roles.manage"},
		ClassIDs: []string{"class-a"},
	}
	if !user.Can("students.read", "school-a", "class-a") {
		t.Fatal("explicit custom-role permission denied for assigned class")
	}
	for _, tc := range []struct{ permission, school, class string }{
		{"students.write", "school-a", "class-a"},
		{"invented.permission", "school-a", "class-a"},
		{"students.read", "school-b", "class-a"},
		{"students.read", "school-a", "class-b"},
		{"users.write", "school-a", ""},
	} {
		if user.Can(tc.permission, tc.school, tc.class) {
			t.Fatalf("unexpected allow permission=%s school=%s class=%s", tc.permission, tc.school, tc.class)
		}
	}
	if user.IsSuperAdmin() {
		t.Fatal("custom role with roles.manage treated as system Superadmin")
	}
}

func TestRoleManagementIsSuperadminOnly(t *testing.T) {
	custom := User{SchoolID: "school-a", Role: "custom", Active: true, Scope: ScopeSchool, PermissionsList: []string{"roles.manage"}}
	if !custom.Can("roles.manage", "school-a", "") || custom.IsSuperAdmin() {
		t.Fatal("custom role permission or identity check is incorrect")
	}
	super := User{SchoolID: "school-a", Role: "super_admin", Active: true, Scope: ScopeSchool, PermissionsList: []string{"users.read", "users.write", "roles.manage"}}
	if !super.IsSuperAdmin() || !super.Can("roles.manage", "school-a", "") {
		t.Fatal("system Superadmin denied role management")
	}
	super.PermissionsList = []string{"users.read", "roles.manage"}
	if super.Can("roles.manage", "school-a", "") {
		t.Fatal("Superadmin bypassed mandatory users.write")
	}
	super.Scope = ScopeAssignedClasses
	if super.Can("users.read", "school-a", "") {
		t.Fatal("Superadmin bypassed mandatory school scope")
	}
}

func TestAssignedClassScope(t *testing.T) {
	user := User{SchoolID: "school-a", Role: "custom", Active: true, Scope: ScopeAssignedClasses, PermissionsList: []string{"attendance.read"}, ClassIDs: []string{"class-a"}}
	if !user.Can("attendance.read", "school-a", "") {
		t.Fatal("role-level permission check should not require a class ID")
	}
	if !user.Can("attendance.read", "school-a", "class-a") {
		t.Fatal("assigned class denied")
	}
	for _, classID := range []string{"class-b"} {
		if user.Can("attendance.read", "school-a", classID) {
			t.Fatalf("unassigned class %q allowed", classID)
		}
	}
	user.Scope = ScopeSchool
	if !user.Can("attendance.read", "school-a", "class-b") {
		t.Fatal("school-scoped role incorrectly constrained by class assignments")
	}
}

func TestPermissionCatalogIsUniqueAndSuperadminBaselineIsComplete(t *testing.T) {
	catalog := PermissionCatalog()
	seen := map[string]bool{}
	for _, permission := range catalog {
		if permission.Key == "" || permission.Label == "" || permission.Description == "" || permission.Group == "" || seen[permission.Key] {
			t.Fatalf("invalid or duplicate catalog entry: %+v", permission)
		}
		seen[permission.Key] = true
	}
	if len(catalog) != 18 {
		t.Fatalf("catalog has %d permissions, want 18", len(catalog))
	}
	for _, required := range []string{"users.read", "users.write", "roles.manage"} {
		if !seen[required] {
			t.Fatalf("mandatory Superadmin permission %q missing from catalog", required)
		}
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

func TestPermissionsNotAssignedToRoleCannotAuthorizeUser(t *testing.T) {
	u := User{SchoolID: "school-a", Role: "operator", Active: true, Scope: ScopeSchool, PermissionsList: []string{"devices.health"}}
	for _, permission := range []string{"reports.export", "biometrics.revoke", "consents.manage", "attendance.approve"} {
		if u.Can(permission, "school-a", "") {
			t.Fatalf("operator gained unassigned permission %s", permission)
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

func TestNewPasswordCompositionPolicyWithoutMaximum(t *testing.T) {
	for _, password := range []string{"Abcd123!", "Password1!", "Éé12345!", "Ab12345🔒", " Ab12!a ", "Ab1!" + strings.Repeat("a", 200)} {
		hash, err := HashPassword(password)
		if err != nil || !VerifyPassword(password, hash) {
			t.Fatal("valid password rejected or changed")
		}
	}
	for _, password := range []string{"Ab12!xy", "abcd123!", "ABCD123!", "Abcdefg!", "Abcd1234", "Abcd123 ", "Abcd123é"} {
		if _, err := HashPassword(password); err == nil {
			t.Fatal("password missing a required rule accepted")
		}
	}
}

func TestLegacyPasswordRemainsUsableForLogin(t *testing.T) {
	password := "legacy lower only"
	salt := []byte("0123456789abcdef")
	key := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	hash := hashPrefix + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key)
	if validNewPassword(password) || !VerifyPassword(password, hash) {
		t.Fatal("new password rules broke an existing credential")
	}
}
