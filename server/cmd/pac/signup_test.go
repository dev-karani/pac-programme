package main

import (
	"fmt"
	"github.com/google/uuid"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStudentRegistrationCannotGrantStaffAccess(t *testing.T) {
	a := integrationApp(t)
	for _, entry := range []string{"new", "continuing"} {
		suffix := uuid.NewString()
		body := fmt.Sprintf(`{"Name":"Registration Test","Email":"%s@test.pac.test","Password":"Registration123!","StudentNumber":"%s","ProgrammeID":"30000000-0000-0000-0000-000000000001","CohortID":"40000000-0000-0000-0000-000000000001","StudyMode":"full_time","AdmissionDate":"2026-01-12","EntryPath":"%s","Role":"dean","ScopeType":"institution"}`, suffix, suffix, entry)
		request := httptest.NewRequest("POST", "/api/v1/auth/signup", strings.NewReader(body))
		response := httptest.NewRecorder()
		a.signup(response, request)
		if response.Code != 201 {
			t.Fatalf("registration %d: %s", response.Code, response.Body.String())
		}
		u := loadTestUser(t, a, suffix+"@test.pac.test")
		if len(u.Roles) != 1 || u.Roles[0].Name != "student" || u.Roles[0].ScopeType != "self" || u.Roles[0].ScopeID != u.ID || u.MustChange {
			t.Fatalf("unsafe role assignment: %+v", u)
		}
		var state, path string
		var plans int
		err := a.db.QueryRow(`SELECT e.state,e.entry_path,(SELECT count(*) FROM student_plans WHERE enrolment_id=e.id) FROM enrolments e WHERE e.student_id=$1`, u.ID).Scan(&state, &path, &plans)
		if err != nil || state != "pending_verification" || path != entry || plans != 0 {
			t.Fatalf("unverified account received plan: %s %s %d %v", state, path, plans, err)
		}
		duplicate := httptest.NewRecorder()
		a.signup(duplicate, httptest.NewRequest("POST", "/api/v1/auth/signup", strings.NewReader(body)))
		if duplicate.Code != 409 {
			t.Fatalf("duplicate registration: %d", duplicate.Code)
		}
	}
}
