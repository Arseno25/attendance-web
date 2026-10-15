package auth

import "slices"

type RoleScope string

const (
	ScopeSchool          RoleScope = "school"
	ScopeAssignedClasses RoleScope = "assigned_classes"
)

type Permission struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Group       string `json:"group"`
}

type Role struct {
	ID          string    `json:"id"`
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Scope       RoleScope `json:"scope"`
	IsSystem    bool      `json:"is_system"`
	Permissions []string  `json:"permissions"`
	UserCount   int       `json:"user_count"`
}

type User struct {
	AvatarURL       string    `json:"avatar_url,omitempty"`
	ID              string    `json:"id"`
	SchoolID        string    `json:"school_id"`
	Email           string    `json:"email"`
	FullName        string    `json:"full_name"`
	Role            string    `json:"role"`
	RoleName        string    `json:"role_name"`
	Scope           RoleScope `json:"-"`
	PermissionsList []string  `json:"-"`
	Active          bool      `json:"is_active"`
	ClassIDs        []string  `json:"class_ids,omitempty"`
	PermissionKeys  []string  `json:"permissions,omitempty"`
}

var permissionCatalog = []Permission{
	{Key: "users.read", Label: "Lihat pengguna", Description: "Melihat akun pengguna sekolah.", Group: "Pengguna & keamanan"},
	{Key: "users.write", Label: "Kelola pengguna", Description: "Membuat dan mengubah akun pengguna.", Group: "Pengguna & keamanan"},
	{Key: "roles.manage", Label: "Kelola peran", Description: "Mengatur peran dan akses pengguna.", Group: "Pengguna & keamanan"},
	{Key: "audit.read", Label: "Lihat riwayat aktivitas", Description: "Melihat riwayat aktivitas sekolah.", Group: "Pengguna & keamanan"},
	{Key: "school.write", Label: "Kelola sekolah", Description: "Mengubah pengaturan sekolah.", Group: "Pengguna & keamanan"},
	{Key: "students.read", Label: "Lihat siswa", Description: "Melihat data siswa.", Group: "Akademik & siswa"},
	{Key: "students.write", Label: "Kelola siswa", Description: "Membuat dan mengubah data siswa.", Group: "Akademik & siswa"},
	{Key: "academics.read", Label: "Lihat akademik", Description: "Melihat tahun ajaran, kelas, dan penugasan.", Group: "Akademik & siswa"},
	{Key: "academics.write", Label: "Kelola akademik", Description: "Mengubah struktur dan penugasan akademik.", Group: "Akademik & siswa"},
	{Key: "attendance.read", Label: "Lihat kehadiran", Description: "Melihat sesi dan data kehadiran.", Group: "Kehadiran"},
	{Key: "attendance.manual", Label: "Input manual", Description: "Mengelola daftar siswa dan mencatat kehadiran.", Group: "Kehadiran"},
	{Key: "attendance.approve", Label: "Setujui kehadiran", Description: "Menyetujui perubahan kehadiran.", Group: "Kehadiran"},
	{Key: "corrections.request", Label: "Ajukan koreksi", Description: "Mengajukan koreksi kehadiran.", Group: "Kehadiran"},
	{Key: "reports.export", Label: "Ekspor laporan", Description: "Melihat dan mengekspor laporan.", Group: "Laporan"},
	{Key: "devices.manage", Label: "Kelola perangkat", Description: "Mengelola perangkat absensi.", Group: "Perangkat"},
	{Key: "devices.health", Label: "Status perangkat", Description: "Melihat status kesehatan perangkat.", Group: "Perangkat"},
	{Key: "biometrics.enroll", Label: "Daftarkan wajah", Description: "Mendaftarkan wajah siswa.", Group: "Wajah siswa"},
	{Key: "biometrics.revoke", Label: "Hapus data wajah", Description: "Menghapus pendaftaran wajah siswa.", Group: "Wajah siswa"},
}

func PermissionCatalog() []Permission {
	return append([]Permission(nil), permissionCatalog...)
}

func knownPermission(permission string) bool {
	return slices.ContainsFunc(permissionCatalog, func(item Permission) bool { return item.Key == permission })
}

func (u User) IsSuperAdmin() bool {
	return u.Role == "super_admin"
}

func (u User) Permissions() []string {
	source := u.PermissionsList
	if source == nil {
		source = u.PermissionKeys
	}
	result := make([]string, 0, len(source))
	for _, permission := range source {
		if knownPermission(permission) && !slices.Contains(result, permission) {
			result = append(result, permission)
		}
	}
	return result
}

func (u User) Can(permission, schoolID, classID string) bool {
	effective := u.Permissions()
	required := permission
	if !slices.Contains(effective, required) {
		switch permission {
		case "students.read":
			required = "students.write"
		case "academics.read":
			required = "academics.write"
		case "devices.health":
			required = "devices.manage"
		}
	}
	if !u.Active || u.SchoolID != schoolID || !knownPermission(permission) || !slices.Contains(effective, required) {
		return false
	}
	if permission == "users.read" || permission == "users.write" || permission == "audit.read" || permission == "school.write" {
		if !u.IsSuperAdmin() {
			return false
		}
	}
	if u.IsSuperAdmin() && (u.Scope != ScopeSchool || !slices.Contains(effective, "users.read") || !slices.Contains(effective, "users.write") || !slices.Contains(effective, "roles.manage")) {
		return false
	}
	if u.Scope == ScopeAssignedClasses {
		return classID == "" || slices.Contains(u.ClassIDs, classID)
	}
	return u.Scope == ScopeSchool
}
