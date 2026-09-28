package main

import (
	"net/http"
	"strings"
	"time"
)

func (a *app) registerCoordination(m *http.ServeMux) {
	for p, h := range map[string]http.HandlerFunc{
		"GET /api/v1/meetings/{id}": a.meetingDetail, "POST /api/v1/meetings/{id}/respond": a.meetingRespond,
		"GET /api/v1/workspace/examination": a.examinationWorkspace, "GET /api/v1/committees/{id}": a.committeeDetail,
		"POST /api/v1/defences/{id}/confirm": a.confirmDefence, "POST /api/v1/defences/{id}/report": a.examReport,
		"POST /api/v1/corrections/{id}/submit": a.submitCorrection, "POST /api/v1/enrolments/{id}/complete": a.completeEnrolment,
		"GET /api/v1/workspace/examiners": a.examinerDirectory,
	} {
		m.HandleFunc(p, a.withAuth(h))
	}
}
func (a *app) meetingDetail(w http.ResponseWriter, r *http.Request) {
	var eid string
	_ = a.db.QueryRowContext(r.Context(), `SELECT enrolment_id FROM meetings WHERE id=$1`, r.PathValue("id")).Scan(&eid)
	if !a.academicAccess(r.Context(), current(r), eid) {
		problem(w, 404, "Meeting unavailable", "Outside your access.")
		return
	}
	meeting, _ := a.jsonRows(r.Context(), `SELECT * FROM meetings WHERE id=$1`, r.PathValue("id"))
	notes, _ := a.jsonRows(r.Context(), `SELECT n.*,u.full_name AS author FROM meeting_notes n JOIN users u ON u.id=n.author_id WHERE meeting_id=$1 ORDER BY created_at`, r.PathValue("id"))
	participants, _ := a.jsonRows(r.Context(), `SELECT mp.*,u.full_name FROM meeting_participants mp JOIN users u ON u.id=mp.user_id WHERE meeting_id=$1`, r.PathValue("id"))
	writeJSON(w, 200, map[string]any{"meeting": meeting, "notes": notes, "participants": participants})
}
func (a *app) meetingRespond(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	var eid string
	_ = a.db.QueryRowContext(r.Context(), `SELECT enrolment_id FROM meetings WHERE id=$1`, r.PathValue("id")).Scan(&eid)
	if !a.academicAccess(r.Context(), u, eid) {
		problem(w, 403, "Access denied", "Meeting outside your scope.")
		return
	}
	var in struct {
		Action, Reason, Notes, NoteID string
		ScheduledAt                   time.Time
	}
	if !decode(w, r, &in) {
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	supervisor := a.assignedSupervisor(r.Context(), u, eid)
	switch in.Action {
	case "confirmed", "cancelled", "rescheduled":
		if !supervisor {
			problem(w, 403, "Supervisor action required", "Only an assigned supervisor may confirm or change this meeting.")
			return
		}
		if in.Action == "rescheduled" {
			if in.ScheduledAt.IsZero() || len(in.Reason) < 5 {
				problem(w, 400, "Date and reason required", "Choose the new meeting time and explain the change.")
				return
			}
			_, err = tx.ExecContext(r.Context(), `UPDATE meetings SET scheduled_at=$2,state='proposed',version=version+1 WHERE id=$1`, r.PathValue("id"), in.ScheduledAt)
		} else {
			_, err = tx.ExecContext(r.Context(), `UPDATE meetings SET state=$2,organizer_id=$3,cancellation_reason=NULLIF($4,''),version=version+1 WHERE id=$1`, r.PathValue("id"), in.Action, u.ID, in.Reason)
		}
	case "acknowledged", "change_requested":
		_, err = tx.ExecContext(r.Context(), `INSERT INTO meeting_participants(meeting_id,user_id,response) VALUES($1,$2,$3) ON CONFLICT(meeting_id,user_id) DO UPDATE SET response=excluded.response`, r.PathValue("id"), u.ID, in.Action)
	case "notes":
		if len(in.Notes) < 5 {
			problem(w, 400, "Notes required", "Enter the proposed meeting notes.")
			return
		}
		_, err = tx.ExecContext(r.Context(), `INSERT INTO meeting_notes(meeting_id,author_id,body) VALUES($1,$2,$3)`, r.PathValue("id"), u.ID, in.Notes)
	case "confirm_notes":
		if !supervisor {
			problem(w, 403, "Supervisor action required", "Only an assigned supervisor may confirm meeting notes.")
			return
		}
		_, err = tx.ExecContext(r.Context(), `UPDATE meeting_notes SET state='confirmed' WHERE id=$1 AND meeting_id=$2 AND state IN('proposed','flagged')`, in.NoteID, r.PathValue("id"))
	case "flag":
		if len(in.Reason) < 5 {
			problem(w, 400, "Reason required", "Explain the inaccuracy.")
			return
		}
		_, err = tx.ExecContext(r.Context(), `UPDATE meeting_notes SET state='flagged',flag_reason=$3 WHERE id=$1 AND meeting_id=$2`, in.NoteID, r.PathValue("id"), in.Reason)
	default:
		problem(w, 400, "Unknown action", "Choose a valid meeting action.")
		return
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO meeting_history(meeting_id,actor_id,action,reason) VALUES($1,$2,$3,$4)`, r.PathValue("id"), u.ID, in.Action, in.Reason)
	}
	commitResponse(w, tx, err, map[string]string{"status": "saved"})
}
func (a *app) examinerDirectory(w http.ResponseWriter, r *http.Request) {
	if !requireAny(w, current(r), "coordinator", "hod", "dean", "leadership") {
		return
	}
	a.sendRows(w, r, `SELECT DISTINCT u.id,u.full_name FROM users u JOIN role_assignments ra ON ra.user_id=u.id WHERE u.active AND ra.role IN('examiner','supervisor','hod') ORDER BY u.full_name`)
}
func (a *app) examinationWorkspace(w http.ResponseWriter, r *http.Request) {
	a.sendRows(w, r, `SELECT c.id,c.enrolment_id,c.state,u.full_name AS student,COALESCE((SELECT json_agg(json_build_object('id',cm.id,'user_id',cm.user_id,'name',mu.full_name,'role',cm.member_role,'response',cm.response,'conflict',cm.conflict_state)) FROM committee_members cm JOIN users mu ON mu.id=cm.user_id WHERE cm.committee_id=c.id),'[]') AS members,COALESCE((SELECT json_agg(de) FROM defence_events de WHERE de.committee_id=c.id),'[]') AS defences FROM committees c JOIN enrolments e ON e.id=c.enrolment_id JOIN users u ON u.id=e.student_id JOIN programmes p ON p.id=e.programme_id JOIN departments d ON d.id=p.department_id WHERE `+academicScope+` OR EXISTS(SELECT 1 FROM committee_members cm WHERE cm.committee_id=c.id AND cm.user_id=$1) ORDER BY c.created_at DESC`, current(r).ID)
}
func (a *app) committeeAccess(r *http.Request, id string) bool {
	var eid string
	var member bool
	_ = a.db.QueryRowContext(r.Context(), `SELECT c.enrolment_id,EXISTS(SELECT 1 FROM committee_members WHERE committee_id=c.id AND user_id=$2) FROM committees c WHERE id=$1`, id, current(r).ID).Scan(&eid, &member)
	return member || a.academicAccess(r.Context(), current(r), eid)
}
func (a *app) committeeDetail(w http.ResponseWriter, r *http.Request) {
	if !a.committeeAccess(r, r.PathValue("id")) {
		problem(w, 404, "Committee unavailable", "Outside your access.")
		return
	}
	id := r.PathValue("id")
	windows, _ := a.jsonRows(r.Context(), `SELECT aw.*,u.full_name FROM availability_windows aw JOIN committee_members cm ON cm.id=aw.committee_member_id JOIN users u ON u.id=cm.user_id WHERE cm.committee_id=$1 ORDER BY starts_at`, id)
	outcomes, _ := a.jsonRows(r.Context(), `SELECT eo.* FROM examination_outcomes eo JOIN defence_events de ON de.id=eo.defence_id WHERE de.committee_id=$1`, id)
	corrections, _ := a.jsonRows(r.Context(), `SELECT cr.*,u.full_name AS verifier FROM corrections cr JOIN users u ON u.id=cr.verifier_id JOIN examination_outcomes eo ON eo.id=cr.outcome_id JOIN defence_events de ON de.id=eo.defence_id WHERE de.committee_id=$1`, id)
	reports, _ := a.jsonRows(r.Context(), `SELECT er.id,er.summary,er.submitted_at,u.full_name AS examiner FROM examiner_reports er JOIN users u ON u.id=er.examiner_id JOIN defence_events de ON de.id=er.defence_id WHERE de.committee_id=$1 AND (er.released_at IS NOT NULL OR er.examiner_id=$2 OR EXISTS(SELECT 1 FROM committee_members cm WHERE cm.committee_id=$1 AND cm.user_id=$2))`, id, current(r).ID)
	writeJSON(w, 200, map[string]any{"availability": windows, "outcomes": outcomes, "corrections": corrections, "reports": reports})
}
func (a *app) confirmDefence(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	var member string
	err = tx.QueryRowContext(r.Context(), `SELECT cm.id FROM committee_members cm JOIN defence_events de ON de.committee_id=cm.committee_id WHERE de.id=$1 AND cm.user_id=$2 AND cm.response='accepted' AND cm.conflict_state IN('none','resolved')`, r.PathValue("id"), u.ID).Scan(&member)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO defence_confirmations(defence_id,committee_member_id,confirmed) VALUES($1,$2,true) ON CONFLICT(defence_id,committee_member_id) DO UPDATE SET confirmed=true,responded_at=now()`, r.PathValue("id"), member)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE defence_events de SET state='confirmed' WHERE de.id=$1 AND NOT EXISTS(SELECT 1 FROM committee_members cm WHERE cm.committee_id=de.committee_id AND NOT EXISTS(SELECT 1 FROM defence_confirmations dc WHERE dc.defence_id=de.id AND dc.committee_member_id=cm.id AND dc.confirmed))`, r.PathValue("id"))
	}
	commitResponse(w, tx, err, map[string]string{"status": "confirmation recorded"})
}
func (a *app) examReport(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	var in struct{ Summary string }
	if !decode(w, r, &in) {
		return
	}
	if len(strings.TrimSpace(in.Summary)) < 20 {
		problem(w, 400, "Report required", "Provide a substantive examination report.")
		return
	}
	var id string
	err := a.db.QueryRowContext(r.Context(), `INSERT INTO examiner_reports(defence_id,examiner_id,report_key,summary) SELECT de.id,$2,'recorded-text-report',$3 FROM defence_events de JOIN committee_members cm ON cm.committee_id=de.committee_id WHERE de.id=$1 AND cm.user_id=$2 AND cm.member_role IN('internal_examiner','external_examiner') AND cm.response='accepted' AND cm.conflict_state IN('none','resolved') RETURNING id`, r.PathValue("id"), u.ID, in.Summary).Scan(&id)
	if err != nil {
		problem(w, 409, "Report not submitted", "Only assigned examiners may submit once; contact administration for corrections.")
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}
func (a *app) submitCorrection(w http.ResponseWriter, r *http.Request) {
	var in struct{ Evidence string }
	if !decode(w, r, &in) {
		return
	}
	if len(in.Evidence) < 10 {
		problem(w, 400, "Evidence required", "Describe the correction and identify the revised submitted document.")
		return
	}
	var id string
	err := a.db.QueryRowContext(r.Context(), `UPDATE corrections cr SET state='submitted',evidence_key=$3 WHERE cr.id=$1 AND cr.state IN('open','returned') AND EXISTS(SELECT 1 FROM examination_outcomes eo JOIN defence_events de ON de.id=eo.defence_id JOIN committees c ON c.id=de.committee_id JOIN enrolments e ON e.id=c.enrolment_id WHERE eo.id=cr.outcome_id AND e.student_id=$2) RETURNING id`, r.PathValue("id"), current(r).ID, in.Evidence).Scan(&id)
	if err != nil {
		problem(w, 409, "Correction unavailable", "Only the student may submit open or returned corrections.")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "submitted"})
}
func (a *app) completeEnrolment(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !a.canManageEnrolment(r.Context(), u, r.PathValue("id")) {
		problem(w, 403, "Access denied", "Enrolment outside your scope.")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(r.Context(), `UPDATE enrolments e SET state='completed' WHERE e.id=$1 AND e.state='active' AND EXISTS(SELECT 1 FROM student_plans sp WHERE sp.enrolment_id=e.id) AND NOT EXISTS(SELECT 1 FROM student_plans sp JOIN milestone_instances mi ON mi.plan_id=sp.id WHERE sp.enrolment_id=e.id AND mi.state<>'approved') AND NOT EXISTS(SELECT 1 FROM corrections cr JOIN examination_outcomes eo ON eo.id=cr.outcome_id JOIN defence_events de ON de.id=eo.defence_id JOIN committees c ON c.id=de.committee_id WHERE c.enrolment_id=e.id AND cr.state<>'verified') RETURNING id`, r.PathValue("id")).Scan(&id)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE student_plans SET completed_at=now() WHERE enrolment_id=$1`, id)
	}
	if err == nil {
		err = auditTx(r.Context(), tx, u.ID, "enrolment", id, "completed", "All required milestones, corrections and deposit sign-off completed")
	}
	commitResponse(w, tx, err, map[string]string{"status": "completed"})
}
