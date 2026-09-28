package main

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func callAs(t *testing.T, a *app, email, method, path, body, id string, h http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	u := loadTestUser(t, a, email)
	r := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(context.WithValue(context.Background(), userKey, u))
	r.SetPathValue("id", id)
	w := httptest.NewRecorder()
	h(w, r)
	return w
}
func TestCaseDetailsRequireAssignment(t *testing.T) {
	a := integrationApp(t)
	id := "90000000-0000-0000-0000-000000000015"
	for _, email := range []string{"dean@demo.pac.test", "admin@demo.pac.test", "supervisor@demo.pac.test"} {
		w := callAs(t, a, email, "GET", "/cases/"+id, "", id, a.caseDetail)
		if w.Code != 404 {
			t.Fatalf("%s: %d %s", email, w.Code, w.Body.String())
		}
	}
	w := callAs(t, a, "support@demo.pac.test", "GET", "/cases/"+id, "", id, a.caseDetail)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "welfare narrative") {
		t.Fatal(w.Body.String())
	}
}
func TestRequestOpensAndReplyPersists(t *testing.T) {
	a := integrationApp(t)
	id := "90000000-0000-0000-0000-000000000001"
	w := callAs(t, a, "student@demo.pac.test", "POST", "/cases/"+id+"/message", `{"Body":"Please confirm the next academic step."}`, id, a.caseMessage)
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	w = callAs(t, a, "coordinator@demo.pac.test", "GET", "/cases/"+id, "", id, a.caseDetail)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "next academic step") {
		t.Fatal(w.Body.String())
	}
}
func TestBaselineCannotCrossEnrolments(t *testing.T) {
	a := integrationApp(t)
	w := callAs(t, a, "dean@demo.pac.test", "POST", "/baseline", `{"MilestoneID":"00000000-0000-0000-0000-000000000099","Explanation":"Verified historical completion","DateKnown":false}`, "60000000-0000-0000-0000-000000000001", a.addBaseline)
	if w.Code != 403 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
}
func TestDifferentAvailabilityWindowsOverlap(t *testing.T) {
	a := integrationApp(t)
	id := "87000000-0000-0000-0000-000000000001"
	w := callAs(t, a, "examiner@demo.pac.test", "GET", "/overlaps", "", id, a.availabilityOverlaps)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var slots []map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &slots)
	if len(slots) == 0 {
		t.Fatal("Different overlapping windows must produce a shared slot")
	}
	w = callAs(t, a, "newstudent@demo.pac.test", "GET", "/overlaps", "", id, a.availabilityOverlaps)
	if w.Code != 403 {
		t.Fatal("Unassigned student accessed committee availability")
	}
}
func TestReportScopeAndCSVFilters(t *testing.T) {
	a := integrationApp(t)
	w := callAs(t, a, "dean@demo.pac.test", "GET", "/report?format=csv&state=completed", "", "", a.workspaceReports)
	if w.Code != 200 || strings.Contains(w.Body.String(), "Lydia") {
		t.Fatalf("School B leaked: %s", w.Body.String())
	}
	w = callAs(t, a, "leadership@demo.pac.test", "GET", "/report?format=csv&state=completed", "", "", a.workspaceReports)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Lydia") {
		t.Fatal("Leadership filtered report missing completed student")
	}
}
func TestConceptRevisionAndAcademicAuthority(t *testing.T) {
	a := integrationApp(t)
	idStudent, eid := uuid.NewString(), uuid.NewString()
	email := "concept-test-" + idStudent + "@demo.pac.test"
	_, err := a.db.Exec(`INSERT INTO users(id,email,full_name,password_hash,must_change_password) VALUES($1,$2,'Concept integration student','integration-test-only',false)`, idStudent, email)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.db.Exec(`INSERT INTO enrolments(id,student_id,programme_id,cohort_id,state,admission_date,study_mode) VALUES($1,$2,'30000000-0000-0000-0000-000000000001','40000000-0000-0000-0000-000000000001','active',current_date,'full_time')`, eid, idStudent)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.db.Exec(`INSERT INTO supervision_assignments(enrolment_id,supervisor_id,position,effective_from,created_by) VALUES($1,'50000000-0000-0000-0000-000000000002','primary',now(),'50000000-0000-0000-0000-000000000003')`, eid)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"Title":"County services adoption study","Problem":"Citizens experience inconsistent access to digital county services.","Objectives":"Identify adoption barriers and evaluate service outcomes.","Methodology":"Mixed methods interviews and a structured survey."}`
	w := callAs(t, a, email, "POST", "/concepts", body, "", a.submitConcept)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var out map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	id := out["id"]
	w = callAs(t, a, "dean@demo.pac.test", "POST", "/decision", `{"Decision":"approved","Feedback":"Approved by oversight"}`, id, a.decideConcept)
	if w.Code != 403 {
		t.Fatal("Dean must not become academic reviewer")
	}
	w = callAs(t, a, "supervisor@demo.pac.test", "POST", "/decision", `{"Decision":"changes_requested","Feedback":"Narrow the sampling frame before approval."}`, id, a.decideConcept)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = callAs(t, a, email, "POST", "/concepts", body, "", a.submitConcept)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	w = callAs(t, a, "supervisor@demo.pac.test", "POST", "/decision", `{"Decision":"declined","Feedback":"Test revision round closed with feedback."}`, out["id"], a.decideConcept)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
}
