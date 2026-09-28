package main

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestDefenceReportsOutcomeAndCorrections(t *testing.T) {
	a := integrationApp(t)
	committee := "87000000-0000-0000-0000-000000000001"
	body := fmt.Sprintf(`{"CommitteeID":"%s","SubmissionVersionID":"86100000-0000-0000-0000-000000000001","StartsAt":"2029-06-20T08:30:00Z","Duration":30,"Venue":"Lifecycle test seminar room"}`, committee)
	w := callAs(t, a, "dean@demo.pac.test", "POST", "/defences", body, "", a.scheduleDefence)
	if w.Code != 201 {
		t.Fatalf("schedule: %d %s", w.Code, w.Body.String())
	}
	var result map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	id := result["id"]
	// A second reservation cannot use the same committee members at that time.
	w = callAs(t, a, "dean@demo.pac.test", "POST", "/defences", body, "", a.scheduleDefence)
	if w.Code != 409 {
		t.Fatalf("overlapping event accepted: %d", w.Code)
	}
	for _, email := range []string{"supervisor@demo.pac.test", "examiner@demo.pac.test", "supervisor2@demo.pac.test"} {
		w = callAs(t, a, email, "POST", "/confirm", `{}`, id, a.confirmDefence)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	outcome := `{"Outcome":"corrections_required","Summary":"Examination passed subject to a documented methods clarification.","Corrections":[{"Title":"Clarify participant sampling","DueDate":"2029-07-01","VerifierID":"50000000-0000-0000-0000-000000000010"}]}`
	w = callAs(t, a, "dean@demo.pac.test", "POST", "/outcome", outcome, id, a.recordOutcome)
	if w.Code != 409 {
		t.Fatalf("outcome accepted before reports: %d", w.Code)
	}
	for _, email := range []string{"examiner@demo.pac.test", "supervisor2@demo.pac.test"} {
		w = callAs(t, a, email, "POST", "/report", `{"Summary":"The research demonstrates a sound contribution. The sampling description requires clarification."}`, id, a.examReport)
		if w.Code != 201 {
			t.Fatal(w.Body.String())
		}
	}
	w = callAs(t, a, "dean@demo.pac.test", "POST", "/outcome", outcome, id, a.recordOutcome)
	if w.Code != 201 {
		t.Fatalf("outcome: %s", w.Body.String())
	}
	var correction string
	if err := a.db.QueryRow(`SELECT cr.id FROM corrections cr JOIN examination_outcomes eo ON eo.id=cr.outcome_id WHERE eo.defence_id=$1`, id).Scan(&correction); err != nil {
		t.Fatal(err)
	}
	w = callAs(t, a, "defence@demo.pac.test", "POST", "/correction", `{"Evidence":"Revised methods clarify the selection of research participants, chapter 3, pages 12–15."}`, correction, a.submitCorrection)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = callAs(t, a, "supervisor@demo.pac.test", "POST", "/verify", `{"Decision":"verified"}`, correction, a.verifyCorrection)
	if w.Code != 409 {
		t.Fatal("unassigned verifier accepted")
	}
	w = callAs(t, a, "examiner@demo.pac.test", "POST", "/verify", `{"Decision":"verified"}`, correction, a.verifyCorrection)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
}
