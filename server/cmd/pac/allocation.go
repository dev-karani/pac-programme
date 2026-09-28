package main

import (
	"database/sql"
	"net/http"
	"strings"
)

func (a *app) allocationExceptions(w http.ResponseWriter, r *http.Request) {
	a.sendRows(w, r, `SELECT x.*,u.full_name AS supervisor,s.full_name AS student FROM allocation_exceptions x JOIN enrolments e ON e.id=x.enrolment_id JOIN users s ON s.id=e.student_id JOIN users u ON u.id=x.supervisor_id JOIN programmes p ON p.id=e.programme_id JOIN departments d ON d.id=p.department_id WHERE `+academicScope+` ORDER BY x.id`, current(r).ID)
}
func (a *app) recordAllocationException(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	var in struct{ EnrolmentID, SupervisorID, Kind, Reason, Decision string }
	if !decode(w, r, &in) {
		return
	}
	if !requireAny(w, u, "hod", "dean", "leadership") || !a.canManageEnrolment(r.Context(), u, in.EnrolmentID) {
		problem(w, 403, "Access denied", "A scoped HOD, dean or graduate-school leader must authorize an allocation exception.")
		return
	}
	if len(strings.TrimSpace(in.Reason)) < 12 || (in.Kind != "capacity" && in.Kind != "eligibility") || (in.Decision != "approved" && in.Decision != "declined") {
		problem(w, 400, "Exception details required", "Choose capacity or eligibility, a decision and a substantive reason.")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "Unavailable", "Try again.")
		return
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(r.Context(), `INSERT INTO allocation_exceptions(enrolment_id,supervisor_id,kind,reason,state,requested_by,decided_by,decided_at) SELECT $1,$2,$3,$4,$5,$6,$6,now() WHERE EXISTS(SELECT 1 FROM supervisor_profiles sp JOIN users u ON u.id=sp.user_id WHERE sp.user_id=$2 AND u.active) RETURNING id`, in.EnrolmentID, in.SupervisorID, in.Kind, in.Reason, in.Decision, u.ID).Scan(&id)
	if err == nil {
		err = auditTx(r.Context(), tx, u.ID, "allocation_exception", id, in.Decision, "Recorded scoped allocation exception: "+in.Kind)
	}
	commitResponse(w, tx, err, map[string]string{"id": id})
}
func allocationAllowed(r *http.Request, tx *sql.Tx, eid, sid string) (bool, error) {
	var ok bool
	err := tx.QueryRowContext(r.Context(), `SELECT u.active AND (EXISTS(SELECT 1 FROM enrolments e JOIN supervisor_programme_eligibility pe ON pe.programme_id=e.programme_id WHERE e.id=$1 AND pe.supervisor_id=$2 AND pe.active) OR EXISTS(SELECT 1 FROM allocation_exceptions WHERE enrolment_id=$1 AND supervisor_id=$2 AND kind='eligibility' AND state='approved')) FROM users u WHERE u.id=$2`, eid, sid).Scan(&ok)
	return ok, err
}
func capacityException(r *http.Request, tx *sql.Tx, eid, sid string) bool {
	var ok bool
	_ = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM allocation_exceptions WHERE enrolment_id=$1 AND supervisor_id=$2 AND kind='capacity' AND state='approved')`, eid, sid).Scan(&ok)
	return ok
}
