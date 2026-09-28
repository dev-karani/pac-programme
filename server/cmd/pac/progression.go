package main

import (
	"context"
	"database/sql"
)

// Keep approved history intact and activate only explicitly unblocked work.
func advancePlan(ctx context.Context, tx *sql.Tx, eid string) error {
	_, err := tx.ExecContext(ctx, `UPDATE milestone_instances mi SET state='in_progress',version=version+1 FROM student_plans sp WHERE sp.id=mi.plan_id AND sp.enrolment_id=$1 AND mi.state='not_started' AND NOT EXISTS(SELECT 1 FROM milestone_dependencies dep JOIN milestone_definitions required ON required.id=dep.depends_on_id WHERE dep.milestone_id=mi.definition_id AND NOT EXISTS(SELECT 1 FROM milestone_instances pre JOIN milestone_definitions pd ON pd.id=pre.definition_id WHERE pre.plan_id=mi.plan_id AND pd.position=required.position AND pre.state='approved'))`, eid)
	return err
}
func generateActivities(ctx context.Context, tx *sql.Tx, eid string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO activity_instances(milestone_id,definition_id,owner_id,baseline_due,current_due,state) SELECT mi.id,ad.id,e.student_id,sp.started_on+ad.due_offset_days,sp.started_on+ad.due_offset_days,'in_progress' FROM enrolments e JOIN student_plans sp ON sp.enrolment_id=e.id JOIN milestone_instances mi ON mi.plan_id=sp.id JOIN activity_definitions ad ON ad.milestone_id=mi.definition_id WHERE e.id=$1 ON CONFLICT DO NOTHING`, eid)
	return err
}
