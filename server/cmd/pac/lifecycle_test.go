package main

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func registeredStudent(t *testing.T, a *app, entry string) (string, string) {
	t.Helper()
	suffix := uuid.NewString()
	email := suffix + "@lifecycle.test"
	body := fmt.Sprintf(`{"Name":"Lifecycle Student","Email":"%s","Password":"Registration123!","StudentNumber":"%s","ProgrammeID":"30000000-0000-0000-0000-000000000001","CohortID":"40000000-0000-0000-0000-000000000001","StudyMode":"full_time","AdmissionDate":"2026-01-12","EntryPath":"%s"}`, email, suffix, entry)
	w := httptest.NewRecorder()
	a.signup(w, httptest.NewRequest("POST", "/signup", strings.NewReader(body)))
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var eid string
	if err := a.db.QueryRow(`SELECT e.id FROM enrolments e JOIN users u ON u.id=e.student_id WHERE u.email=$1`, email).Scan(&eid); err != nil {
		t.Fatal(err)
	}
	w = callAs(t, a, "coordinator@demo.pac.test", "POST", "/verify", `{"Decision":"verified","Note":"Admission record checked","PlanStart":"2026-09-01T00:00:00Z"}`, eid, a.verifyEnrolment)
	if w.Code != 200 {
		t.Fatalf("verify: %s", w.Body.String())
	}
	return email, eid
}
func testSupervisor(t *testing.T, a *app, capacity int) (string, string) {
	t.Helper()
	id := uuid.NewString()
	email := id + "@supervisor.test"
	for _, q := range []string{
		`INSERT INTO users(id,email,full_name,password_hash,must_change_password) VALUES($1,$2,'Test Supervisor','test-only',false)`,
		`INSERT INTO role_assignments(user_id,role,scope_type,scope_id) SELECT id,'supervisor','self',id FROM users WHERE id=$1 AND email=$2`,
	} {
		if _, err := a.db.Exec(q, id, email); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.db.Exec(`INSERT INTO supervisor_profiles(user_id,home_department_id,description,capacity,accepting_students) VALUES($1,'20000000-0000-0000-0000-000000000001','Test supervisor',$2,true)`, id, capacity); err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.Exec(`INSERT INTO supervisor_programme_eligibility(supervisor_id,programme_id) VALUES($1,'30000000-0000-0000-0000-000000000001')`, id); err != nil {
		t.Fatal(err)
	}
	return email, id
}
func requestSupervision(t *testing.T, a *app, student, sid, position string) string {
	t.Helper()
	w := callAs(t, a, student, "POST", "/request", fmt.Sprintf(`{"SupervisorID":"%s","Position":"%s","Note":"Research supervision request"}`, sid, position), "", a.requestSupervisor)
	if w.Code != 201 {
		t.Fatalf("request: %d %s", w.Code, w.Body.String())
	}
	var v map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &v)
	return v["id"]
}
func TestNewAndContinuingStudentLifecycle(t *testing.T) {
	a := integrationApp(t)
	student, eid := registeredStudent(t, a, "continuing")
	var mid string
	if err := a.db.QueryRow(`SELECT mi.id FROM milestone_instances mi JOIN student_plans sp ON sp.id=mi.plan_id JOIN milestone_definitions md ON md.id=mi.definition_id WHERE sp.enrolment_id=$1 ORDER BY md.position LIMIT 1`, eid).Scan(&mid); err != nil {
		t.Fatal(err)
	}
	w := callAs(t, a, "coordinator@demo.pac.test", "POST", "/baseline", fmt.Sprintf(`{"MilestoneID":"%s","DateKnown":false,"Explanation":"Verified historical orientation against departmental records"}`, mid), eid, a.addBaseline)
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	for _, position := range []string{"primary", "secondary"} {
		supervisor, sid := testSupervisor(t, a, 5)
		rid := requestSupervision(t, a, student, sid, position)
		w = callAs(t, a, supervisor, "POST", "/respond", `{"Decision":"accepted","Reason":"Research expertise matches"}`, rid, a.respondSupervision)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		w = callAs(t, a, "coordinator@demo.pac.test", "POST", "/allocate", `{}`, rid, a.allocateSupervisor)
		if w.Code != 201 {
			t.Fatal(w.Body.String())
		}
	}
	var n int
	if err := a.db.QueryRow(`SELECT count(DISTINCT supervisor_id) FROM supervision_assignments WHERE enrolment_id=$1 AND effective_to IS NULL`, eid).Scan(&n); err != nil || n != 2 {
		t.Fatalf("distinct supervisors: %d %v", n, err)
	}
}
func TestConcurrentReservationsCannotExceedCapacity(t *testing.T) {
	a := integrationApp(t)
	supervisor, sid := testSupervisor(t, a, 1)
	first, _ := registeredStudent(t, a, "new")
	second, _ := registeredStudent(t, a, "new")
	ids := []string{requestSupervision(t, a, first, sid, "primary"), requestSupervision(t, a, second, sid, "primary")}
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			w := callAs(t, a, supervisor, "POST", "/respond", `{"Decision":"accepted","Reason":"Research expertise matches"}`, id, a.respondSupervision)
			codes <- w.Code
		}(id)
	}
	wg.Wait()
	close(codes)
	success := 0
	for code := range codes {
		if code == 200 {
			success++
		} else if code != 409 {
			t.Fatalf("unexpected response: %d", code)
		}
	}
	if success != 1 {
		t.Fatalf("capacity 1 admitted %d reservations", success)
	}
}
func TestStudentMeetingConfirmationAndAction(t *testing.T) {
	a := integrationApp(t)
	student, eid := registeredStudent(t, a, "new")
	supervisor, sid := testSupervisor(t, a, 2)
	rid := requestSupervision(t, a, student, sid, "primary")
	w := callAs(t, a, supervisor, "POST", "/respond", `{"Decision":"accepted","Reason":"Research expertise matches"}`, rid, a.respondSupervision)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = callAs(t, a, "coordinator@demo.pac.test", "POST", "/allocate", `{}`, rid, a.allocateSupervisor)
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	w = callAs(t, a, student, "POST", "/meetings", fmt.Sprintf(`{"EnrolmentID":"%s","Title":"Concept discussion","Purpose":"Discuss research objectives","ScheduledAt":"2026-09-27T10:00:00Z","Duration":60,"Location":"Research office"}`, eid), "", a.createMeeting)
	if w.Code != 201 {
		t.Fatalf("meeting: %s", w.Body.String())
	}
	var result map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	id := result["id"]
	w = callAs(t, a, supervisor, "POST", "/respond", `{"Action":"confirmed","Reason":"Time agreed with student"}`, id, a.meetingRespond)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	u := loadTestUser(t, a, student)
	w = callAs(t, a, supervisor, "POST", "/record", fmt.Sprintf(`{"Held":true,"ActualAt":"2026-09-27T10:00:00Z","Notes":"Research objectives were clarified.","Attended":["%s","%s"],"Actions":[{"OwnerID":"%s","Title":"Revise research objectives","DueAt":"%s"}]}`, u.ID, sid, u.ID, time.Now().Add(48*time.Hour).Format(time.RFC3339)), id, a.recordMeeting)
	if w.Code != 200 {
		t.Fatalf("record: %s", w.Body.String())
	}
	w = callAs(t, a, student, "GET", "/tasks", "", "", a.tasks)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Revise research objectives") {
		t.Fatal(w.Body.String())
	}
}
func TestAllReportKindsUseScope(t *testing.T) {
	a := integrationApp(t)
	for _, kind := range []string{"progress", "overdue", "allocation", "leave", "completed", "reviews", "cases"} {
		for _, format := range []string{"json", "csv"} {
			w := callAs(t, a, "dean@demo.pac.test", "GET", "/reports?kind="+kind+"&format="+format, "", "", a.workspaceReports)
			if w.Code != 200 || strings.Contains(w.Body.String(), "Lydia") {
				t.Fatalf("%s/%s: %d %s", kind, format, w.Code, w.Body.String())
			}
		}
	}
}
