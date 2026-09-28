package main

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestLeaveExtensionAndReturnPreserveBaseline(t *testing.T) {
	a := integrationApp(t)
	student, eid := registeredStudent(t, a, "new")
	var mid string
	var baseline, original time.Time
	if err := a.db.QueryRow(`SELECT mi.id,mi.baseline_due,mi.current_due FROM milestone_instances mi JOIN student_plans sp ON sp.id=mi.plan_id JOIN milestone_definitions md ON md.id=mi.definition_id WHERE sp.enrolment_id=$1 ORDER BY md.position LIMIT 1`, eid).Scan(&mid, &baseline, &original); err != nil {
		t.Fatal(err)
	}

	var activity string
	if err := a.db.QueryRow(`WITH def AS(INSERT INTO activity_definitions(milestone_id,title,activity_type,owner_role,position,required) SELECT definition_id,'Schedule test checkpoint','checkpoint','student',999,false FROM milestone_instances WHERE id=$1 RETURNING id) INSERT INTO activity_instances(milestone_id,definition_id,baseline_due,current_due,state) SELECT $1,id,$2,$2,'in_progress' FROM def RETURNING id`, mid, original).Scan(&activity); err != nil {
		t.Fatal(err)
	}
	next := original.AddDate(0, 0, 14).Format("2006-01-02")
	w := callAs(t, a, student, "POST", "/extensions", fmt.Sprintf(`{"MilestoneID":"%s","RequestedDate":"%s","Reason":"Approved research access requires another fortnight"}`, mid, next), "", a.requestExtension)
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var result map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	w = callAs(t, a, "coordinator@demo.pac.test", "POST", "/extension/decide", `{"Decision":"approved","Reason":"Verified research access delay"}`, result["id"], a.decideExtension)
	if w.Code != 200 {
		t.Fatalf("extension: %s", w.Body.String())
	}
	var unchanged, current time.Time
	_ = a.db.QueryRow(`SELECT baseline_due,current_due FROM milestone_instances WHERE id=$1`, mid).Scan(&unchanged, &current)
	if !unchanged.Equal(baseline) || current.Format("2006-01-02") != next {
		t.Fatal("extension changed baseline or did not update current due")
	}

	var activityDue, activityBaseline time.Time
	if err := a.db.QueryRow(`SELECT current_due,baseline_due FROM activity_instances WHERE id=$1`, activity).Scan(&activityDue, &activityBaseline); err != nil {
		t.Fatal(err)
	}
	if activityDue.Format("2006-01-02") != next || !activityBaseline.Equal(original) {
		t.Fatal("extension failed to preserve baseline and shift activity date")
	}
	w = callAs(t, a, student, "POST", "/leave", `{"StartDate":"2026-09-10","EndDate":"2026-09-20","ReturnDate":"2026-09-21","Category":"personal","Explanation":"Private leave explanation"}`, "", a.requestLeave)
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	w = callAs(t, a, "dean@demo.pac.test", "POST", "/leave/decide", `{"Decision":"approved","Reason":"Dates and return arrangements approved"}`, result["id"], a.decideLeave)
	if w.Code != 200 {
		t.Fatalf("leave: %s", w.Body.String())
	}
	var state string
	_ = a.db.QueryRow(`SELECT state FROM enrolments WHERE id=$1`, eid).Scan(&state)
	if state != "approved_leave" {
		t.Fatal(state)
	}
	var revisions int
	_ = a.db.QueryRow(`SELECT count(*) FROM plan_revisions pr JOIN student_plans sp ON sp.id=pr.plan_id WHERE sp.enrolment_id=$1`, eid).Scan(&revisions)
	if revisions != 2 {
		t.Fatalf("expected extension and leave revision, got %d", revisions)
	}
	w = callAs(t, a, "coordinator@demo.pac.test", "POST", "/return", `{"Reason":"Student has returned and programme office confirmed attendance"}`, eid, a.returnFromLeave)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	_ = a.db.QueryRow(`SELECT state FROM enrolments WHERE id=$1`, eid).Scan(&state)
	if state != "active" {
		t.Fatal(state)
	}
}
