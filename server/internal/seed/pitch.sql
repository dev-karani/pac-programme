-- Explicit opt-in fictional pitch data. Safe to rerun; never run on real students.
BEGIN;
-- Brian represents a university-provisioned account, not public registration.
UPDATE enrolments SET state='active',verification_status='verified',plan_start_date=current_date,
 verification_note='Fictional university-provisioned demo account; ERP is not connected.'
 WHERE id='60000000-0000-0000-0000-000000000011' AND state='pending_verification';
INSERT INTO student_plans(id,enrolment_id,template_id,started_on,planned_completion)
SELECT '72000000-0000-0000-0000-000000000011',id,'70000000-0000-0000-0000-000000000003',plan_start_date,plan_start_date+1095
FROM enrolments WHERE id='60000000-0000-0000-0000-000000000011' AND state='active' ON CONFLICT DO NOTHING;
INSERT INTO milestone_instances(plan_id,definition_id,baseline_start,baseline_due,current_start,current_due,state)
SELECT sp.id,md.id,sp.started_on+md.start_days,sp.started_on+md.end_days,sp.started_on+md.start_days,sp.started_on+md.end_days,
 CASE WHEN md.position=1 THEN 'in_progress'::workflow_state ELSE 'not_started'::workflow_state END
FROM student_plans sp JOIN milestone_definitions md ON md.template_id=sp.template_id
WHERE sp.enrolment_id='60000000-0000-0000-0000-000000000011' ON CONFLICT DO NOTHING;

INSERT INTO activity_definitions(id,milestone_id,title,activity_type,owner_role,instructions,required,position) VALUES
('a1000000-0000-0000-0000-000000000001','71000000-0000-0000-0000-000000000001','Identify a practical information-systems problem','checkpoint','student','Fictional exercise: identify a county digital-service problem, affected users and why it matters. Read the concept guide in Resources.',true,1),
('a1000000-0000-0000-0000-000000000002','71000000-0000-0000-0000-000000000001','Discuss scope with your supervisory team','checkpoint','student','Agree on a feasible scope, intended beneficiaries and supervision expectations.',true,2),
('a1000000-0000-0000-0000-000000000003','71000000-0000-0000-0000-000000000002','Prepare the first concept paper','checkpoint','student','Describe the problem, objectives and a feasible method. Use the example concept guide; consider digital inclusion as a development context.',true,1),
('a1000000-0000-0000-0000-000000000004','71000000-0000-0000-0000-000000000002','Address supervisor feedback on research scope','checkpoint','student','Narrow the study to county e-service adoption and explain the target population. Review the approved concept and discussion.',true,2),
('a1000000-0000-0000-0000-000000000005','71000000-0000-0000-0000-000000000003','Build a literature comparison matrix','checkpoint','student','Compare at least eight relevant studies by question, method, findings and research gap. Keep citation details for each source.',false,1),
('a1000000-0000-0000-0000-000000000006','71000000-0000-0000-0000-000000000003','Refine sampling and data-protection plan','checkpoint','student','Explain the sampling frame, consent process, anonymisation and storage. Bring unresolved questions to the methodology meeting.',false,2),
('a1000000-0000-0000-0000-000000000007','71200000-0000-0000-0000-000000000001','Prepare your initial research idea','checkpoint','student','After requesting supervision, outline a problem, objectives and proposed methodology using the concept guide. Submit it in Research concepts.',false,1)
ON CONFLICT DO NOTHING;
INSERT INTO activity_instances(milestone_id,definition_id,owner_id,baseline_due,current_due,state,completed_at)
SELECT mi.id,ad.id,e.student_id,mi.baseline_due,mi.current_due,
 CASE WHEN mi.state='approved' OR ad.id='a1000000-0000-0000-0000-000000000005' THEN 'approved'::workflow_state ELSE 'in_progress'::workflow_state END,
 CASE WHEN mi.state='approved' THEN mi.approved_at WHEN ad.id='a1000000-0000-0000-0000-000000000005' THEN now()-interval '2 days' END
FROM activity_definitions ad JOIN milestone_instances mi ON mi.definition_id=ad.milestone_id JOIN student_plans sp ON sp.id=mi.plan_id JOIN enrolments e ON e.id=sp.enrolment_id
WHERE ad.id::text LIKE 'a1000000-%' AND e.id IN('60000000-0000-0000-0000-000000000001','60000000-0000-0000-0000-000000000011')
ON CONFLICT DO NOTHING;
INSERT INTO resources(id,milestone_definition_id,title,kind,url,created_by)
SELECT ('a2000000-0000-0000-0000-'||lpad(position::text,12,'0'))::uuid,id,'Concept and proposal preparation guide (fictional demo)','guidance','/research-guide.html','50000000-0000-0000-0000-000000000003'
FROM milestone_definitions WHERE template_id='70000000-0000-0000-0000-000000000001' AND position<=3 ON CONFLICT DO NOTHING;
INSERT INTO resources(id,milestone_definition_id,title,kind,url,created_by) VALUES
('a2000000-0000-0000-0000-000000000011','71200000-0000-0000-0000-000000000001','First research concept guide (fictional demo)','guidance','/research-guide.html','50000000-0000-0000-0000-000000000003') ON CONFLICT DO NOTHING;
INSERT INTO research_concepts(id,enrolment_id,title,problem,objectives,methodology,version,state,feedback,reviewer_id,submitted_at,decided_at)
SELECT 'a3000000-0000-0000-0000-000000000001',e.id,'A national platform for every public service','Citizens encounter fragmented digital public services, but the proposed national scope is too broad for this study.','Explore barriers to using digital public services.','A proposed nationwide survey of public-service users.',1,'declined','The scope is not feasible for this programme. Narrow the question to one county and a clearly defined service.','50000000-0000-0000-0000-000000000002','2025-10-10 10:00+03','2025-10-14 14:00+03'
FROM enrolments e WHERE e.id='60000000-0000-0000-0000-000000000001' AND NOT EXISTS(SELECT 1 FROM research_concepts WHERE enrolment_id=e.id) ON CONFLICT DO NOTHING;
INSERT INTO research_concepts(id,enrolment_id,title,problem,objectives,methodology,version,state,feedback,reviewer_id,submitted_at,decided_at)
SELECT 'a3000000-0000-0000-0000-000000000002',e.id,'Digital service adoption among small businesses in a Kenyan county','Small businesses may underuse county e-services because of usability, connectivity and trust barriers. This fictional study examines those barriers in one county.','Identify adoption barriers; evaluate perceived usability and trust; recommend feasible service improvements.','A mixed-methods study combining a scoped user survey with interviews. Sampling and consent details will be refined in the proposal.',2,'approved','The revised scope is feasible. Develop the sampling frame, literature matrix and data-protection plan in the proposal.','50000000-0000-0000-0000-000000000002','2025-11-24 10:00+03','2025-12-11 14:10+03'
FROM enrolments e WHERE e.id='60000000-0000-0000-0000-000000000001' AND EXISTS(SELECT 1 FROM research_concepts WHERE id='a3000000-0000-0000-0000-000000000001') AND NOT EXISTS(SELECT 1 FROM research_concepts WHERE enrolment_id=e.id AND version=2) ON CONFLICT DO NOTHING;
INSERT INTO submissions(id,milestone_id,student_id,current_version,state) VALUES
('a4000000-0000-0000-0000-000000000001','73000000-0000-0000-0000-000000000002','50000000-0000-0000-0000-000000000001',1,'approved') ON CONFLICT DO NOTHING;
INSERT INTO submission_versions(id,submission_id,version,storage_key,original_name,content_type,size_bytes,idempotency_key,submitted_at)
SELECT 'a5000000-0000-0000-0000-000000000001','a4000000-0000-0000-0000-000000000001',1,'demo-concept-document','illustrative-academic-document-demo.pdf','application/pdf',size_bytes,'pitch-concept-example','2025-12-10 10:00+03'
FROM submission_versions WHERE id='86100000-0000-0000-0000-000000000001' ON CONFLICT DO NOTHING;
INSERT INTO comments(id,enrolment_id,milestone_id,submission_version_id,author_id,body,created_at) VALUES
('a6000000-0000-0000-0000-000000000001','60000000-0000-0000-0000-000000000001','73000000-0000-0000-0000-000000000002','a5000000-0000-0000-0000-000000000001','50000000-0000-0000-0000-000000000001','Fictional demo: I have narrowed the concept to digital-service adoption by small businesses. The attached PDF is an illustrative document, not a real student paper.','2025-12-10 10:05+03'),
('a6000000-0000-0000-0000-000000000002','60000000-0000-0000-0000-000000000001','73000000-0000-0000-0000-000000000002','a5000000-0000-0000-0000-000000000001','50000000-0000-0000-0000-000000000002','The concept is approved. Carry the focused objectives into the proposal and explain how participants will give informed consent.','2025-12-11 14:15+03'),
('a6000000-0000-0000-0000-000000000003','60000000-0000-0000-0000-000000000001','73000000-0000-0000-0000-000000000003',NULL,'50000000-0000-0000-0000-000000000002','For our next meeting, bring your literature matrix and a draft sampling frame. Keep the research question consistent with the approved concept.',now()-interval '1 day')
ON CONFLICT DO NOTHING;
INSERT INTO notifications(recipient_id,dedupe_key,title,urgency,target_view,milestone_id) VALUES
('50000000-0000-0000-0000-000000000008','pitch-overview','Demo: Amina has completed her concept and is preparing her proposal','info','journey','73000000-0000-0000-0000-000000000003'),
('50000000-0000-0000-0000-000000000011','pitch-start','Your university-issued account is ready. Request your supervisor.','info','supervision',NULL)
ON CONFLICT DO NOTHING;
COMMIT;
