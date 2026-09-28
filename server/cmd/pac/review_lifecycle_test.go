package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http/httptest"
	"pac-progress/server/internal/seed"
	"testing"
)

func TestImmutableVersionsAndBothSupervisorApproval(t *testing.T) {
	a := integrationApp(t)
	student, eid := registeredStudent(t, a, "new")
	supervisors := []string{}
	sids := []string{}
	for _, pos := range []string{"primary", "secondary"} {
		email, sid := testSupervisor(t, a, 4)
		supervisors = append(supervisors, email)
		sids = append(sids, sid)
		rid := requestSupervision(t, a, student, sid, pos)
		w := callAs(t, a, email, "POST", "/respond", `{"Decision":"accepted","Reason":"Research expertise matches"}`, rid, a.respondSupervision)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		w = callAs(t, a, "coordinator@demo.pac.test", "POST", "/allocate", `{}`, rid, a.allocateSupervisor)
		if w.Code != 201 {
			t.Fatal(w.Body.String())
		}
	}
	var mid, tid, definition string
	if err := a.db.QueryRow(`SELECT mi.id FROM milestone_instances mi JOIN student_plans sp ON sp.id=mi.plan_id JOIN milestone_definitions md ON md.id=mi.definition_id WHERE sp.enrolment_id=$1 ORDER BY md.position LIMIT 1`, eid).Scan(&mid); err != nil {
		t.Fatal(err)
	}
	if err := a.db.QueryRow(`INSERT INTO programme_templates(programme_id,name,study_mode,version,state) SELECT programme_id,'Both approval test','full_time',max(version)+1,'draft' FROM programme_templates WHERE programme_id='30000000-0000-0000-0000-000000000001' GROUP BY programme_id RETURNING id`).Scan(&tid); err != nil {
		t.Fatal(err)
	}
	if err := a.db.QueryRow(`INSERT INTO milestone_definitions(template_id,title,stage_label,position,start_days,end_days,requires_final_signoff,similarity_required,approval_mode) VALUES($1,'Both review test','Research',1,0,30,true,false,'both') RETURNING id`, tid).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.Exec(`UPDATE milestone_instances SET definition_id=$2 WHERE id=$1`, mid, definition); err != nil {
		t.Fatal(err)
	}
	upload := func() {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, _ := writer.CreateFormFile("document", "test-research.pdf")
		_, _ = part.Write(seed.DemoDocument)
		writer.Close()
		u := loadTestUser(t, a, student)
		r := httptest.NewRequest("POST", "/submit", &body).WithContext(context.WithValue(context.Background(), userKey, u))
		r.SetPathValue("id", mid)
		r.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()
		a.submitDocument(w, r)
		if w.Code != 201 {
			t.Fatalf("upload: %d %s", w.Code, w.Body.String())
		}
	}
	assignment := func(index int) string {
		var id string
		err := a.db.QueryRow(`SELECT ra.id FROM review_assignments ra JOIN submission_versions sv ON sv.id=ra.submission_version_id JOIN submissions s ON s.id=sv.submission_id WHERE s.milestone_id=$1 AND sv.version=s.current_version AND ra.reviewer_id=$2`, mid, sids[index]).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	upload()
	w := callAs(t, a, supervisors[0], "POST", "/review", `{"Decision":"changes_requested","Feedback":"Revise the methodology with a clearer sampling frame."}`, assignment(0), a.reviewDecision)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	oldAssignment := assignment(1)
	upload()
	w = callAs(t, a, supervisors[1], "POST", "/review", `{"Decision":"approved","Feedback":"This old version must not be accepted."}`, oldAssignment, a.reviewDecision)
	if w.Code != 409 {
		t.Fatalf("stale round accepted: %d", w.Code)
	}
	for i, email := range supervisors {
		w = callAs(t, a, email, "POST", "/review", `{"Decision":"approved","Feedback":"The revised methods and objectives are acceptable."}`, assignment(i), a.reviewDecision)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		var state string
		_ = a.db.QueryRow(`SELECT state FROM milestone_instances WHERE id=$1`, mid).Scan(&state)
		if i == 0 && state == "approved" {
			t.Fatal("one approval incorrectly completed both-review milestone")
		}
		if i == 1 && state != "approved" {
			t.Fatalf("both decisions did not complete milestone: %s", state)
		}
	}
	w = callAs(t, a, student, "GET", "/milestone", "", mid, a.milestoneDetail)
	var detail map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &detail)
	if len(detail["versions"].([]any)) != 2 {
		t.Fatal(fmt.Sprint(detail["versions"]))
	}
}
