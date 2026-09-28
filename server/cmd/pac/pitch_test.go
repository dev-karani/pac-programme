package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProvisionedOnlyRegistration(t *testing.T) {
	a := &app{}
	w := httptest.NewRecorder()
	a.provisionedOnly(w, httptest.NewRequest("POST", "/api/v1/auth/signup", strings.NewReader("{}")))
	if w.Code != 403 {
		t.Fatalf("public signup available: %d", w.Code)
	}
}
func TestDiscussionNotificationsAndVersionScope(t *testing.T) {
	a := integrationApp(t)
	mid := "73000000-0000-0000-0000-000000000003"
	w := callAs(t, a, "student@demo.pac.test", "POST", "/comments", `{"Body":"Please clarify the sampling frame for this proposal."}`, mid, a.addComment)
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var count int
	if err := a.db.QueryRow(`SELECT count(*) FROM notifications WHERE recipient_id='50000000-0000-0000-0000-000000000002' AND milestone_id=$1 AND dedupe_key LIKE 'comment:%'`, mid).Scan(&count); err != nil || count == 0 {
		t.Fatalf("missing supervisor notification: %v", err)
	}
	w = callAs(t, a, "student@demo.pac.test", "POST", "/comments", `{"Body":"Wrong submission reference","VersionID":"86100000-0000-0000-0000-000000000001"}`, mid, a.addComment)
	if w.Code != 400 {
		t.Fatal("cross-student submission version accepted")
	}
	w = callAs(t, a, "admin@demo.pac.test", "POST", "/comments", `{"Body":"Unauthorized academic comment"}`, mid, a.addComment)
	if w.Code != 403 {
		t.Fatal("admin gained academic comment access")
	}
}
func TestFollowupDeliveryReplyAndAuthorization(t *testing.T) {
	a := integrationApp(t)
	eid := "60000000-0000-0000-0000-000000000001"
	w := callAs(t, a, "dean@demo.pac.test", "GET", "/contacts", "", eid, a.followupRecipients)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "supervisor@demo.pac.test") {
		t.Fatal(w.Body.String())
	}
	w = callAs(t, a, "dean@demo.pac.test", "POST", "/followups", `{"RecipientID":"50000000-0000-0000-0000-000000000002","Body":"Please follow up on the proposal review this week."}`, eid, a.sendFollowup)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var id string
	if err := a.db.QueryRow(`SELECT id FROM notifications WHERE recipient_id='50000000-0000-0000-0000-000000000002' AND body='Please follow up on the proposal review this week.' ORDER BY updated_at DESC LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	w = callAs(t, a, "student@demo.pac.test", "POST", "/read", "{}", id, a.readNotification)
	if w.Code != 404 {
		t.Fatal("another recipient could mark notification read")
	}
	w = callAs(t, a, "supervisor@demo.pac.test", "POST", "/read", "{}", id, a.readNotification)
	if w.Code != 204 {
		t.Fatal(w.Body.String())
	}
	w = callAs(t, a, "supervisor@demo.pac.test", "POST", "/followups", `{"RecipientID":"50000000-0000-0000-0000-000000000008","Body":"I will complete the review on Friday."}`, eid, a.sendFollowup)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = callAs(t, a, "dean@demo.pac.test", "GET", "/followups", "", eid, a.followups)
	if !strings.Contains(w.Body.String(), "Friday") {
		t.Fatal(w.Body.String())
	}
	w = callAs(t, a, "student@demo.pac.test", "GET", "/followups", "", eid, a.followups)
	if strings.Contains(w.Body.String(), "Friday") {
		t.Fatal("third party read direct conversation")
	}
	w = callAs(t, a, "dean@demo.pac.test", "POST", "/followups", `{"RecipientID":"50000000-0000-0000-0000-000000000006","Body":"Not an academic participant"}`, eid, a.sendFollowup)
	if w.Code != 403 {
		t.Fatal("unauthorized recipient accepted")
	}
	w = callAs(t, a, "dean@demo.pac.test", "GET", "/contacts", "", "60000000-0000-0000-0000-000000000014", a.followupRecipients)
	if w.Code != 403 {
		t.Fatal("cross-school contacts exposed")
	}
}
func TestStaleExtensionCannotOverwriteSchedule(t *testing.T) {
	a := integrationApp(t)
	student, eid := registeredStudent(t, a, "new")
	var mid, due string
	if err := a.db.QueryRow(`SELECT mi.id,to_char(mi.current_due+20,'YYYY-MM-DD') FROM milestone_instances mi JOIN student_plans sp ON sp.id=mi.plan_id WHERE sp.enrolment_id=$1 ORDER BY current_due LIMIT 1`, eid).Scan(&mid, &due); err != nil {
		t.Fatal(err)
	}
	w := callAs(t, a, student, "POST", "/extensions", fmt.Sprintf(`{"MilestoneID":"%s","RequestedDate":"%s","Reason":"Research access delayed"}`, mid, due), "", a.requestExtension)
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var record map[string]string
	json.Unmarshal(w.Body.Bytes(), &record)
	if _, err := a.db.Exec(`UPDATE milestone_instances SET current_due=current_due+5 WHERE id=$1`, mid); err != nil {
		t.Fatal(err)
	}
	w = callAs(t, a, "dean@demo.pac.test", "POST", "/decide", `{"Decision":"approved","Reason":"Grant requested extension"}`, record["id"], a.decideExtension)
	if w.Code != 409 {
		t.Fatal("stale extension overwrote revised schedule", w.Code, w.Body.String())
	}
	w = callAs(t, a, "dean@demo.pac.test", "POST", "/decide", `{"Decision":"declined","Reason":"Schedule changed; request updated dates"}`, record["id"], a.decideExtension)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
}

func TestCompletedActionNotificationsAreHidden(t *testing.T) {
	a := integrationApp(t)
	var id string
	err := a.db.QueryRow(`INSERT INTO action_tasks(enrolment_id,owner_id,title,task_type) VALUES('60000000-0000-0000-0000-000000000001','50000000-0000-0000-0000-000000000001','Temporary notification lifecycle check','pitch_notification_test') RETURNING id`).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	w := callAs(t, a, "student@demo.pac.test", "GET", "/notifications", "", "", a.notifications)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Temporary notification lifecycle check") {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, err = a.db.Exec(`UPDATE action_tasks SET state='completed' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	w = callAs(t, a, "student@demo.pac.test", "GET", "/notifications", "", "", a.notifications)
	if w.Code != 200 || strings.Contains(w.Body.String(), "Temporary notification lifecycle check") {
		t.Fatal("completed task still raises alert", w.Body.String())
	}
}
