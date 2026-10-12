package auth

import "slices"

type User struct {
	ID       string   `json:"id"`
	SchoolID string   `json:"school_id"`
	Email    string   `json:"email"`
	FullName string   `json:"full_name"`
	Role     string   `json:"role"`
	Active   bool     `json:"is_active"`
	Grants   []string `json:"grants"`
	ClassIDs []string `json:"class_ids"`
}

var Grantable = []string{"biometrics.enroll", "biometrics.revoke", "consents.manage", "attendance.manual", "attendance.approve", "reports.export"}

func GrantAllowed(role, permission string) bool {
	if !slices.Contains(Grantable, permission) {
		return false
	}
	if role == "super_admin" || role == "admin" {
		return true
	}
	if role == "operator" {
		return permission == "biometrics.enroll" || permission == "attendance.manual"
	}
	return role == "teacher" && permission == "reports.export"
}

var rolePermissions = map[string][]string{
	"super_admin": {"users.read", "users.write", "audit.read", "school.write", "students.read", "students.write", "academics.read", "academics.write", "attendance.read", "attendance.manual", "attendance.approve", "corrections.request", "reports.export", "devices.manage", "devices.health", "biometrics.enroll", "biometrics.revoke", "consents.manage"},
	"admin":       {"students.read", "students.write", "academics.read", "academics.write", "attendance.read", "attendance.manual", "corrections.request", "reports.export", "devices.manage", "devices.health"},
	"teacher":     {"students.read", "academics.read", "attendance.read", "corrections.request", "reports.export"},
	"operator":    {"devices.health"},
}

func (u User) Permissions() []string {
	result := append([]string{}, rolePermissions[u.Role]...)
	for _, grant := range u.Grants {
		if GrantAllowed(u.Role, grant) && !slices.Contains(result, grant) {
			result = append(result, grant)
		}
	}
	return result
}

func (u User) Can(permission, schoolID, classID string) bool {
	if !u.Active || u.SchoolID != schoolID {
		return false
	}
	if permission == "users.read" || permission == "users.write" || permission == "audit.read" {
		return u.Role == "super_admin"
	}
	if !slices.Contains(u.Permissions(), permission) {
		return false
	}
	if u.Role == "teacher" {
		return classID != "" && slices.Contains(u.ClassIDs, classID)
	}
	if classID != "" && u.Role != "admin" && u.Role != "super_admin" {
		return slices.Contains(u.ClassIDs, classID)
	}
	return true
}
