INSERT INTO institutions (id, name, timezone) VALUES
('00000000-0000-0000-0000-000000000001','PAC University demo','Africa/Nairobi') ON CONFLICT DO NOTHING;
INSERT INTO schools (id,institution_id,name) VALUES
('10000000-0000-0000-0000-000000000001','00000000-0000-0000-0000-000000000001','School of Computing & Engineering'),
('10000000-0000-0000-0000-000000000002','00000000-0000-0000-0000-000000000001','School of Social Sciences') ON CONFLICT DO NOTHING;
INSERT INTO departments (id,school_id,name) VALUES
('20000000-0000-0000-0000-000000000001','10000000-0000-0000-0000-000000000001','Computing & Informatics'),
('20000000-0000-0000-0000-000000000002','10000000-0000-0000-0000-000000000001','Engineering'),
('20000000-0000-0000-0000-000000000003','10000000-0000-0000-0000-000000000002','Governance & Development') ON CONFLICT DO NOTHING;
INSERT INTO programmes (id,department_id,name,code) VALUES
('30000000-0000-0000-0000-000000000001','20000000-0000-0000-0000-000000000001','Master of Science in Information Systems','MSC-IS'),
('30000000-0000-0000-0000-000000000002','20000000-0000-0000-0000-000000000001','Doctor of Philosophy in Computer Science','PHD-CS'),
('30000000-0000-0000-0000-000000000003','20000000-0000-0000-0000-000000000002','Master of Science in Engineering','MSC-ENG'),
('30000000-0000-0000-0000-000000000004','20000000-0000-0000-0000-000000000003','Master of Development Studies','MDS') ON CONFLICT DO NOTHING;
INSERT INTO cohorts (id,programme_id,name,intake_date,study_mode) VALUES
('40000000-0000-0000-0000-000000000001','30000000-0000-0000-0000-000000000001','September 2025','2025-09-01','full_time'),
('40000000-0000-0000-0000-000000000002','30000000-0000-0000-0000-000000000001','January 2026','2026-01-12','part_time'),
('40000000-0000-0000-0000-000000000003','30000000-0000-0000-0000-000000000002','September 2025','2025-09-01','full_time'),
('40000000-0000-0000-0000-000000000004','30000000-0000-0000-0000-000000000002','January 2026','2026-01-12','part_time'),
('40000000-0000-0000-0000-000000000005','30000000-0000-0000-0000-000000000003','September 2025','2025-09-01','full_time'),
('40000000-0000-0000-0000-000000000006','30000000-0000-0000-0000-000000000003','January 2026','2026-01-12','part_time'),
('40000000-0000-0000-0000-000000000007','30000000-0000-0000-0000-000000000004','September 2025','2025-09-01','full_time'),
('40000000-0000-0000-0000-000000000008','30000000-0000-0000-0000-000000000004','January 2026','2026-01-12','part_time') ON CONFLICT DO NOTHING;
INSERT INTO users (id,email,full_name,password_hash,student_number,must_change_password) VALUES
('50000000-0000-0000-0000-000000000001','student@demo.pac.test','Amina Wanjiku',crypt('Demo123!Change',gen_salt('bf')),'PAC/PG/0261',false),
('50000000-0000-0000-0000-000000000002','supervisor@demo.pac.test','Dr. Samuel Otieno',crypt('Demo123!Change',gen_salt('bf')),NULL,false),
('50000000-0000-0000-0000-000000000003','coordinator@demo.pac.test','Dr. Njeri Kamau',crypt('Demo123!Change',gen_salt('bf')),NULL,false),
('50000000-0000-0000-0000-000000000004','hod@demo.pac.test','Prof. David Mutua',crypt('Demo123!Change',gen_salt('bf')),NULL,false),
('50000000-0000-0000-0000-000000000005','support@demo.pac.test','Faith Achieng',crypt('Demo123!Change',gen_salt('bf')),NULL,false),
('50000000-0000-0000-0000-000000000006','admin@demo.pac.test','PAC System Administrator',crypt('Demo123!Change',gen_salt('bf')),NULL,false) ON CONFLICT DO NOTHING;
INSERT INTO role_assignments (user_id,role,scope_type,scope_id) VALUES
('50000000-0000-0000-0000-000000000001','student','self','50000000-0000-0000-0000-000000000001'),
('50000000-0000-0000-0000-000000000002','supervisor','programme','30000000-0000-0000-0000-000000000001'),
('50000000-0000-0000-0000-000000000003','coordinator','programme','30000000-0000-0000-0000-000000000001'),
('50000000-0000-0000-0000-000000000004','hod','department','20000000-0000-0000-0000-000000000001'),
('50000000-0000-0000-0000-000000000005','support','institution','00000000-0000-0000-0000-000000000001'),
('50000000-0000-0000-0000-000000000006','admin','institution','00000000-0000-0000-0000-000000000001') ON CONFLICT DO NOTHING;
INSERT INTO enrolments (id,student_id,programme_id,cohort_id,state,admission_date,study_mode,plan_start_date) VALUES
('60000000-0000-0000-0000-000000000001','50000000-0000-0000-0000-000000000001','30000000-0000-0000-0000-000000000001','40000000-0000-0000-0000-000000000001','active','2025-09-01','full_time','2025-09-08') ON CONFLICT DO NOTHING;
INSERT INTO programme_templates (id,programme_id,study_mode,name,version,state) VALUES
('70000000-0000-0000-0000-000000000001','30000000-0000-0000-0000-000000000001','full_time','Research journey — demonstration policy',1,'published'),
('70000000-0000-0000-0000-000000000002','30000000-0000-0000-0000-000000000002','full_time','Doctoral research journey — demonstration policy',1,'published'),
('70000000-0000-0000-0000-000000000003','30000000-0000-0000-0000-000000000001','part_time','Part-time research journey — demonstration policy',1,'published') ON CONFLICT DO NOTHING;
INSERT INTO milestone_definitions (id,template_id,title,description,stage_label,position,start_days,end_days,requires_final_signoff) VALUES
('71000000-0000-0000-0000-000000000001','70000000-0000-0000-0000-000000000001','Orientation & research concept','Confirm your research direction and supervisory team.','Orientation',1,0,28,false),
('71000000-0000-0000-0000-000000000002','70000000-0000-0000-0000-000000000001','Concept paper','Develop the research problem, objectives and initial literature framing.','Concept',2,29,90,true),
('71000000-0000-0000-0000-000000000003','70000000-0000-0000-0000-000000000001','Research proposal','Prepare the full proposal and methodology for formal approval.','Proposal',3,91,180,true),
('71000000-0000-0000-0000-000000000004','70000000-0000-0000-0000-000000000001','Ethics & permissions','Secure all required ethics and field permissions.','Ethics',4,181,225,true),
('71000000-0000-0000-0000-000000000005','70000000-0000-0000-0000-000000000001','Research & data collection','Carry out approved fieldwork and maintain evidence.','Research',5,226,365,false),
('71000000-0000-0000-0000-000000000006','70000000-0000-0000-0000-000000000001','Thesis writing','Develop and review the complete thesis.','Writing',6,366,540,true),
('71000000-0000-0000-0000-000000000007','70000000-0000-0000-0000-000000000001','Examination & defence','Submit the examination package and complete the defence.','Examination',7,541,630,true),
('71000000-0000-0000-0000-000000000008','70000000-0000-0000-0000-000000000001','Corrections & final deposit','Complete corrections and deposit the final approved copy.','Completion',8,631,690,true) ON CONFLICT DO NOTHING;
INSERT INTO milestone_definitions (id,template_id,title,description,stage_label,position,start_days,end_days,requires_final_signoff) VALUES
('71100000-0000-0000-0000-000000000001','70000000-0000-0000-0000-000000000002','Doctoral orientation & research framing','Establish the doctoral question and supervisory compact.','Orientation',1,0,60,false),
('71100000-0000-0000-0000-000000000002','70000000-0000-0000-0000-000000000002','Candidacy proposal','Complete an original doctoral proposal and candidacy review.','Candidacy',2,61,365,true),
('71100000-0000-0000-0000-000000000003','70000000-0000-0000-0000-000000000002','Doctoral research','Complete fieldwork, analysis and annual progress reviews.','Research',3,366,1095,true),
('71100000-0000-0000-0000-000000000004','70000000-0000-0000-0000-000000000002','Thesis examination & deposit','Submit, defend, correct and deposit the doctoral thesis.','Examination',4,1096,1460,true),
('71200000-0000-0000-0000-000000000001','70000000-0000-0000-0000-000000000003','Part-time orientation','Develop your research concept and confirm supervision arrangements.','Orientation',1,0,45,false),
('71200000-0000-0000-0000-000000000002','70000000-0000-0000-0000-000000000003','Part-time proposal','Develop and secure approval for the research proposal.','Proposal',2,46,270,true),
('71200000-0000-0000-0000-000000000003','70000000-0000-0000-0000-000000000003','Research & thesis development','Complete approved research and develop the thesis.','Research',3,271,900,true),
('71200000-0000-0000-0000-000000000004','70000000-0000-0000-0000-000000000003','Examination, corrections & deposit','Complete examination and final deposit requirements.','Examination',4,901,1095,true) ON CONFLICT DO NOTHING;
INSERT INTO student_plans (id,enrolment_id,template_id,started_on,planned_completion) VALUES
('72000000-0000-0000-0000-000000000001','60000000-0000-0000-0000-000000000001','70000000-0000-0000-0000-000000000001','2025-09-08','2027-07-30') ON CONFLICT DO NOTHING;
UPDATE milestone_definitions SET similarity_required=true WHERE id IN ('71000000-0000-0000-0000-000000000003','71000000-0000-0000-0000-000000000006','71100000-0000-0000-0000-000000000004');
INSERT INTO milestone_instances (id,plan_id,definition_id,baseline_start,baseline_due,current_start,current_due,state,approved_at) VALUES
('73000000-0000-0000-0000-000000000001','72000000-0000-0000-0000-000000000001','71000000-0000-0000-0000-000000000001','2025-09-08','2025-10-06','2025-09-08','2025-10-06','approved','2025-10-02 09:00+03'),
('73000000-0000-0000-0000-000000000002','72000000-0000-0000-0000-000000000001','71000000-0000-0000-0000-000000000002','2025-10-07','2025-12-07','2025-10-07','2025-12-14','approved','2025-12-11 14:10+03'),
('73000000-0000-0000-0000-000000000003','72000000-0000-0000-0000-000000000001','71000000-0000-0000-0000-000000000003','2025-12-08','2026-03-07','2025-12-15','2026-10-03','in_progress',NULL),
('73000000-0000-0000-0000-000000000004','72000000-0000-0000-0000-000000000001','71000000-0000-0000-0000-000000000004','2026-03-08','2026-04-21','2026-10-04','2026-11-18','not_started',NULL),
('73000000-0000-0000-0000-000000000005','72000000-0000-0000-0000-000000000001','71000000-0000-0000-0000-000000000005','2026-04-22','2026-09-08','2026-11-19','2027-04-07','not_started',NULL),
('73000000-0000-0000-0000-000000000006','72000000-0000-0000-0000-000000000001','71000000-0000-0000-0000-000000000006','2026-09-09','2027-03-02','2027-04-08','2027-09-29','not_started',NULL),
('73000000-0000-0000-0000-000000000007','72000000-0000-0000-0000-000000000001','71000000-0000-0000-0000-000000000007','2027-03-03','2027-05-31','2027-09-30','2027-12-28','not_started',NULL),
('73000000-0000-0000-0000-000000000008','72000000-0000-0000-0000-000000000001','71000000-0000-0000-0000-000000000008','2027-06-01','2027-07-30','2027-12-29','2028-02-26','not_started',NULL) ON CONFLICT DO NOTHING;
INSERT INTO action_tasks (id,enrolment_id,milestone_id,owner_id,title,task_type,due_at) VALUES
('80000000-0000-0000-0000-000000000001','60000000-0000-0000-0000-000000000001','73000000-0000-0000-0000-000000000003','50000000-0000-0000-0000-000000000001','Submit revised research proposal','submission','2026-10-03 23:59+03'),
('80000000-0000-0000-0000-000000000002','60000000-0000-0000-0000-000000000001','73000000-0000-0000-0000-000000000003','50000000-0000-0000-0000-000000000001','Confirm meeting notes','meeting_follow_up','2026-09-29 17:00+03') ON CONFLICT DO NOTHING;
INSERT INTO support_cases (id,enrolment_id,category,summary,restricted,owner_id,status,next_follow_up) VALUES
('90000000-0000-0000-0000-000000000001','60000000-0000-0000-0000-000000000001','academic guidance','Clarification requested on the methodology review sequence.',false,'50000000-0000-0000-0000-000000000003','in_progress','2026-09-30') ON CONFLICT DO NOTHING;
INSERT INTO notifications (recipient_id,dedupe_key,title,urgency) VALUES
('50000000-0000-0000-0000-000000000001','meeting-notes-8001','Meeting notes are ready to confirm','info'),
('50000000-0000-0000-0000-000000000001','proposal-due-8001','Research proposal is due soon','warning') ON CONFLICT DO NOTHING;

-- Full-release demonstration actors and workflow states.
INSERT INTO milestone_dependencies(milestone_id,depends_on_id)
SELECT md.id,prior.id FROM milestone_definitions md JOIN milestone_definitions prior ON prior.template_id=md.template_id AND prior.position=md.position-1 ON CONFLICT DO NOTHING;
INSERT INTO users (id,email,full_name,password_hash,student_number,must_change_password) VALUES
('50000000-0000-0000-0000-000000000007','supervisor2@demo.pac.test','Dr. Grace Mwangi',crypt('Demo123!Change',gen_salt('bf')),NULL,false),
('50000000-0000-0000-0000-000000000008','dean@demo.pac.test','Prof. Ruth Naliaka',crypt('Demo123!Change',gen_salt('bf')),NULL,false),
('50000000-0000-0000-0000-000000000009','leadership@demo.pac.test','Prof. Peter Kariuki',crypt('Demo123!Change',gen_salt('bf')),NULL,false),
('50000000-0000-0000-0000-000000000010','examiner@demo.pac.test','Dr. Jane Kilonzo',crypt('Demo123!Change',gen_salt('bf')),NULL,false),
('50000000-0000-0000-0000-000000000011','newstudent@demo.pac.test','Brian Kiptoo',crypt('Demo123!Change',gen_salt('bf')),'PAC/PG/0301',false),
('50000000-0000-0000-0000-000000000012','leave@demo.pac.test','Mercy Atieno',crypt('Demo123!Change',gen_salt('bf')),'PAC/PG/0302',false),
('50000000-0000-0000-0000-000000000013','defence@demo.pac.test','Kevin Maina',crypt('Demo123!Change',gen_salt('bf')),'PAC/PG/0303',false),
('50000000-0000-0000-0000-000000000014','completed@demo.pac.test','Lydia Wambui',crypt('Demo123!Change',gen_salt('bf')),'PAC/PG/0304',false),
('50000000-0000-0000-0000-000000000015','welfare@demo.pac.test','Irene Chebet',crypt('Demo123!Change',gen_salt('bf')),'PAC/PG/0305',false) ON CONFLICT DO NOTHING;
INSERT INTO role_assignments(user_id,role,scope_type,scope_id) VALUES
('50000000-0000-0000-0000-000000000007','supervisor','programme','30000000-0000-0000-0000-000000000001'),
('50000000-0000-0000-0000-000000000007','supervisor','programme','30000000-0000-0000-0000-000000000004'),
('50000000-0000-0000-0000-000000000008','dean','school','10000000-0000-0000-0000-000000000001'),
('50000000-0000-0000-0000-000000000009','leadership','institution','00000000-0000-0000-0000-000000000001'),
('50000000-0000-0000-0000-000000000010','examiner','self','50000000-0000-0000-0000-000000000010'),
('50000000-0000-0000-0000-000000000011','student','self','50000000-0000-0000-0000-000000000011'),
('50000000-0000-0000-0000-000000000012','student','self','50000000-0000-0000-0000-000000000012'),
('50000000-0000-0000-0000-000000000013','student','self','50000000-0000-0000-0000-000000000013'),
('50000000-0000-0000-0000-000000000014','student','self','50000000-0000-0000-0000-000000000014'),
('50000000-0000-0000-0000-000000000015','student','self','50000000-0000-0000-0000-000000000015') ON CONFLICT DO NOTHING;
INSERT INTO enrolments(id,student_id,programme_id,cohort_id,state,admission_date,study_mode,plan_start_date,verification_status) VALUES
('60000000-0000-0000-0000-000000000011','50000000-0000-0000-0000-000000000011','30000000-0000-0000-0000-000000000001','40000000-0000-0000-0000-000000000002','pending_verification','2026-01-12','part_time',NULL,'submitted'),
('60000000-0000-0000-0000-000000000012','50000000-0000-0000-0000-000000000012','30000000-0000-0000-0000-000000000001','40000000-0000-0000-0000-000000000001','approved_leave','2025-09-01','full_time','2025-09-08','verified'),
('60000000-0000-0000-0000-000000000013','50000000-0000-0000-0000-000000000013','30000000-0000-0000-0000-000000000002','40000000-0000-0000-0000-000000000003','active','2025-09-01','full_time','2025-09-08','verified'),
('60000000-0000-0000-0000-000000000014','50000000-0000-0000-0000-000000000014','30000000-0000-0000-0000-000000000004','40000000-0000-0000-0000-000000000007','completed','2025-09-01','full_time','2025-09-08','verified'),
('60000000-0000-0000-0000-000000000015','50000000-0000-0000-0000-000000000015','30000000-0000-0000-0000-000000000001','40000000-0000-0000-0000-000000000001','active','2025-09-01','full_time','2025-09-08','verified') ON CONFLICT DO NOTHING;

INSERT INTO supervisor_profiles(user_id,home_department_id,description,expertise_tags,accepting_students,capacity) VALUES
('50000000-0000-0000-0000-000000000002','20000000-0000-0000-0000-000000000001','Information systems, digital public services and research methods.',ARRAY['information systems','digital government','mixed methods'],true,5),
('50000000-0000-0000-0000-000000000007','20000000-0000-0000-0000-000000000003','Development informatics and technology adoption across sectors.',ARRAY['development informatics','technology adoption'],false,1) ON CONFLICT DO NOTHING;
INSERT INTO supervisor_programme_eligibility(supervisor_id,programme_id) VALUES
('50000000-0000-0000-0000-000000000002','30000000-0000-0000-0000-000000000001'),
('50000000-0000-0000-0000-000000000002','30000000-0000-0000-0000-000000000002'),
('50000000-0000-0000-0000-000000000007','30000000-0000-0000-0000-000000000001'),
('50000000-0000-0000-0000-000000000007','30000000-0000-0000-0000-000000000004') ON CONFLICT DO NOTHING;
INSERT INTO supervision_requests(id,enrolment_id,supervisor_id,position,state,requested_at,responded_at,reservation_expires_at) VALUES
('81000000-0000-0000-0000-000000000001','60000000-0000-0000-0000-000000000001','50000000-0000-0000-0000-000000000002','primary','confirmed',now()-interval '180 days',now()-interval '178 days',now()-interval '171 days'),
('81000000-0000-0000-0000-000000000002','60000000-0000-0000-0000-000000000001','50000000-0000-0000-0000-000000000007','secondary','confirmed',now()-interval '180 days',now()-interval '177 days',now()-interval '170 days') ON CONFLICT DO NOTHING;
INSERT INTO supervision_assignments(id,enrolment_id,supervisor_id,position,effective_from,request_id,created_by) VALUES
('82000000-0000-0000-0000-000000000001','60000000-0000-0000-0000-000000000001','50000000-0000-0000-0000-000000000002','primary',now()-interval '170 days','81000000-0000-0000-0000-000000000001','50000000-0000-0000-0000-000000000003'),
('82000000-0000-0000-0000-000000000002','60000000-0000-0000-0000-000000000001','50000000-0000-0000-0000-000000000007','secondary',now()-interval '170 days','81000000-0000-0000-0000-000000000002','50000000-0000-0000-0000-000000000003'),
('82000000-0000-0000-0000-000000000003','60000000-0000-0000-0000-000000000015','50000000-0000-0000-0000-000000000007','primary',now()-interval '80 days',NULL,'50000000-0000-0000-0000-000000000003') ON CONFLICT DO NOTHING;
INSERT INTO topics(id,supervisor_id,title,description,tags,capacity,closing_date,state) VALUES
('83000000-0000-0000-0000-000000000001','50000000-0000-0000-0000-000000000002','Digital service adoption in county governments','Study organizational and citizen factors affecting adoption of digital public services.',ARRAY['digital government','adoption'],3,current_date+30,'published'),
('83000000-0000-0000-0000-000000000002','50000000-0000-0000-0000-000000000007','Responsible data use in development programmes','Examine governance practices for responsible use of beneficiary data.',ARRAY['data governance','development'],2,current_date+21,'published') ON CONFLICT DO NOTHING;
INSERT INTO topic_programmes(topic_id,programme_id) VALUES
('83000000-0000-0000-0000-000000000001','30000000-0000-0000-0000-000000000001'),
('83000000-0000-0000-0000-000000000002','30000000-0000-0000-0000-000000000001'),
('83000000-0000-0000-0000-000000000002','30000000-0000-0000-0000-000000000004') ON CONFLICT DO NOTHING;
INSERT INTO meetings(id,enrolment_id,milestone_id,title,purpose,scheduled_at,duration_minutes,meeting_link,organizer_id,state) VALUES
('84000000-0000-0000-0000-000000000001','60000000-0000-0000-0000-000000000001','73000000-0000-0000-0000-000000000003','Proposal methodology review','Resolve sampling and analysis questions.',now()+interval '6 days',60,'https://meet.example.invalid/demo','50000000-0000-0000-0000-000000000002','confirmed') ON CONFLICT DO NOTHING;
INSERT INTO meeting_participants(meeting_id,user_id,response) VALUES
('84000000-0000-0000-0000-000000000001','50000000-0000-0000-0000-000000000001','confirmed'),
('84000000-0000-0000-0000-000000000001','50000000-0000-0000-0000-000000000002','confirmed') ON CONFLICT DO NOTHING;
INSERT INTO support_cases(id,enrolment_id,category,summary,restricted,owner_id,status,next_follow_up) VALUES
('90000000-0000-0000-0000-000000000015','60000000-0000-0000-0000-000000000015','personal/welfare','Restricted demonstration welfare narrative.',true,'50000000-0000-0000-0000-000000000005','in_progress',current_date+3) ON CONFLICT DO NOTHING;
INSERT INTO case_access_grants(case_id,user_id,granted_by) VALUES
('90000000-0000-0000-0000-000000000015','50000000-0000-0000-0000-000000000005','50000000-0000-0000-0000-000000000003') ON CONFLICT DO NOTHING;
INSERT INTO leave_requests(id,enrolment_id,start_date,end_date,reason_category,proposed_return_date,state,decided_by,decision_reason,decided_at) VALUES
('85000000-0000-0000-0000-000000000001','60000000-0000-0000-0000-000000000012',current_date-30,current_date+30,'personal',current_date+31,'approved','50000000-0000-0000-0000-000000000004','Approved demonstration leave.',now()-interval '30 days') ON CONFLICT DO NOTHING;

INSERT INTO student_plans(id,enrolment_id,template_id,started_on,planned_completion) VALUES
('72000000-0000-0000-0000-000000000013','60000000-0000-0000-0000-000000000013','70000000-0000-0000-0000-000000000002','2025-09-08','2029-09-07') ON CONFLICT DO NOTHING;
INSERT INTO milestone_instances(id,plan_id,definition_id,baseline_start,baseline_due,current_start,current_due,state,approved_at) VALUES
('73100000-0000-0000-0000-000000000001','72000000-0000-0000-0000-000000000013','71100000-0000-0000-0000-000000000001','2025-09-08','2025-11-07','2025-09-08','2025-11-07','approved','2025-11-01'),
('73100000-0000-0000-0000-000000000002','72000000-0000-0000-0000-000000000013','71100000-0000-0000-0000-000000000002','2025-11-08','2026-09-08','2025-11-08','2026-09-08','approved','2026-09-01'),
('73100000-0000-0000-0000-000000000003','72000000-0000-0000-0000-000000000013','71100000-0000-0000-0000-000000000003','2026-09-09','2028-09-07','2026-09-09','2028-09-07','approved','2028-08-30'),
('73100000-0000-0000-0000-000000000004','72000000-0000-0000-0000-000000000013','71100000-0000-0000-0000-000000000004','2028-09-08','2029-09-07','2028-09-08','2029-09-07','awaiting_review',NULL) ON CONFLICT DO NOTHING;
INSERT INTO submissions(id,milestone_id,student_id,current_version,state,approval_mode) VALUES
('86000000-0000-0000-0000-000000000001','73100000-0000-0000-0000-000000000004','50000000-0000-0000-0000-000000000013',1,'approved','both') ON CONFLICT DO NOTHING;
INSERT INTO submission_versions(id,submission_id,version,storage_key,original_name,content_type,size_bytes,submitted_at,idempotency_key) VALUES
('86100000-0000-0000-0000-000000000001','86000000-0000-0000-0000-000000000001',1,'demo-examination-package','doctoral-thesis-demo.pdf','application/pdf',429,now()-interval '30 days','demo-defence-version') ON CONFLICT DO NOTHING;
INSERT INTO committees(id,enrolment_id,state,created_by) VALUES
('87000000-0000-0000-0000-000000000001','60000000-0000-0000-0000-000000000013','confirmed','50000000-0000-0000-0000-000000000003') ON CONFLICT DO NOTHING;
INSERT INTO committee_members(id,committee_id,user_id,member_role,response,conflict_state) VALUES
('87100000-0000-0000-0000-000000000001','87000000-0000-0000-0000-000000000001','50000000-0000-0000-0000-000000000002','chair','accepted','none'),
('87100000-0000-0000-0000-000000000002','87000000-0000-0000-0000-000000000001','50000000-0000-0000-0000-000000000010','internal_examiner','accepted','none'),
('87100000-0000-0000-0000-000000000003','87000000-0000-0000-0000-000000000001','50000000-0000-0000-0000-000000000007','external_examiner','accepted','resolved') ON CONFLICT DO NOTHING;
INSERT INTO availability_windows(id,committee_member_id,starts_at,ends_at) VALUES
('87300000-0000-0000-0000-000000000001','87100000-0000-0000-0000-000000000001','2029-06-20 09:00+03','2029-06-20 12:00+03'),
('87300000-0000-0000-0000-000000000002','87100000-0000-0000-0000-000000000002','2029-06-20 10:00+03','2029-06-20 13:00+03'),
('87300000-0000-0000-0000-000000000003','87100000-0000-0000-0000-000000000003','2029-06-20 10:30+03','2029-06-20 12:00+03') ON CONFLICT DO NOTHING;
INSERT INTO defence_events(id,committee_id,submission_version_id,starts_at,duration_minutes,venue,state,created_by) VALUES
('87200000-0000-0000-0000-000000000001','87000000-0000-0000-0000-000000000001','86100000-0000-0000-0000-000000000001','2029-06-20 10:30+03',60,'Graduate Seminar Room','confirmed','50000000-0000-0000-0000-000000000003') ON CONFLICT DO NOTHING;
