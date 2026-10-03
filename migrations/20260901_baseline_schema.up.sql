-- Baseline schema for the tables owned by the GORM models in
-- internal/repository (Models()). Generated from GORM's Postgres dialect, so
-- it matches what AutoMigrate creates. Idempotent: safe on a database that was
-- already created by AutoMigrate. Run before 20260928_bug_dashboard_additive.
-- qa_documents and project_qa_assignments are created by that later migration.
BEGIN;

CREATE TABLE IF NOT EXISTS "projects" ("id" text,"jira_init_id" text,"jira_init_key" text,"name" text,"status" text,"health" text,"qa_owner" text,"qase_project_code" text,"project_size" text,"staging_mttt_minutes" bigint,"beta_mttt_minutes" bigint,"qase_total_cases" bigint,"qase_total_suites" bigint,"staging_start_at" timestamptz,"staging_end_at" timestamptz,"beta_start_at" timestamptz,"beta_end_at" timestamptz,"created_at" timestamptz,"updated_at" timestamptz,PRIMARY KEY ("id"));
CREATE INDEX IF NOT EXISTS "idx_projects_qase_project_code" ON "projects" ("qase_project_code");
CREATE UNIQUE INDEX IF NOT EXISTS "idx_projects_jira_init_key" ON "projects" ("jira_init_key");
CREATE UNIQUE INDEX IF NOT EXISTS "idx_projects_jira_init_id" ON "projects" ("jira_init_id");
CREATE TABLE IF NOT EXISTS "members" ("id" text,"name" text,"jira_email" text,"jira_account_id" text,"qase_member_id" text,"weekly_capacity_hours" decimal,"active" boolean NOT NULL,PRIMARY KEY ("id"));
CREATE INDEX IF NOT EXISTS "idx_members_qase_display_name" ON "members" ("qase_member_id");
CREATE INDEX IF NOT EXISTS "idx_members_jira_account_id" ON "members" ("jira_account_id");
CREATE TABLE IF NOT EXISTS "allocations" ("id" text,"project_id" text,"member_id" text,"week_start" timestamptz,"planned_hours" decimal,PRIMARY KEY ("id"));
CREATE INDEX IF NOT EXISTS "idx_allocations_week_start" ON "allocations" ("week_start");
CREATE INDEX IF NOT EXISTS "idx_allocations_member_id" ON "allocations" ("member_id");
CREATE INDEX IF NOT EXISTS "idx_allocations_project_id" ON "allocations" ("project_id");
CREATE TABLE IF NOT EXISTS "jira_issues" ("id" text,"external_id" text,"key" text,"project_id" text,"summary" text,"issue_type" text,"severity" text,"status" text,"creator" text,"reporter" text,"assignee" text,"parent_key" text,"environment" text,"created_at" timestamptz,"active" boolean NOT NULL DEFAULT true,"sync_scope" text,"source_updated_at" timestamptz,"fetched_at" timestamptz,PRIMARY KEY ("id"));
CREATE INDEX IF NOT EXISTS "idx_jira_issues_sync_scope" ON "jira_issues" ("sync_scope");
CREATE INDEX IF NOT EXISTS "idx_jira_issues_active" ON "jira_issues" ("active");
CREATE INDEX IF NOT EXISTS "idx_jira_issues_environment" ON "jira_issues" ("environment");
CREATE INDEX IF NOT EXISTS "idx_jira_issues_parent_key" ON "jira_issues" ("parent_key");
CREATE INDEX IF NOT EXISTS "idx_jira_issues_issue_type" ON "jira_issues" ("issue_type");
CREATE INDEX IF NOT EXISTS "idx_jira_issues_project_id" ON "jira_issues" ("project_id");
CREATE INDEX IF NOT EXISTS "idx_jira_issues_key" ON "jira_issues" ("key");
CREATE UNIQUE INDEX IF NOT EXISTS "idx_jira_issues_external_id" ON "jira_issues" ("external_id");
CREATE TABLE IF NOT EXISTS "jira_init_qas" ("init_key" text,"jira_account_id" text,"display_name" text,"init_name" text,"init_status" text,"synced_at" timestamptz,PRIMARY KEY ("init_key","jira_account_id"));
CREATE TABLE IF NOT EXISTS "qase_cases" ("id" text,"project_code" text,"case_id" bigint,"title" text,"pic_names" text,"tester_name" text,"tester_android_name" text,"tester_ios_name" text,"beta_tester_name" text,"updated_at" timestamptz,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX IF NOT EXISTS "ux_case" ON "qase_cases" ("project_code","case_id");
CREATE TABLE IF NOT EXISTS "qase_runs" ("id" text,"project_code" text,"run_id" bigint,"title" text,"status" text,"platform" text,"environment" text,"active" boolean,"started_at" timestamptz,"finished_at" timestamptz,"fetched_at" timestamptz,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX IF NOT EXISTS "ux_run" ON "qase_runs" ("project_code","run_id");
CREATE TABLE IF NOT EXISTS "qase_run_cases" ("id" text,"project_code" text,"run_id" bigint,"case_id" bigint,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX IF NOT EXISTS "ux_run_case" ON "qase_run_cases" ("project_code","run_id","case_id");
CREATE TABLE IF NOT EXISTS "qase_results" ("id" text,"project_code" text,"result_id" text,"run_id" bigint,"case_id" bigint,"configuration_key" text,"status" text,"member_id" text,"environment" text,"platform" text,"tester_name" text,"started_at" timestamptz,"ended_at" timestamptz,"fetched_at" timestamptz,PRIMARY KEY ("id"));
CREATE INDEX IF NOT EXISTS "idx_qase_results_ended_at" ON "qase_results" ("ended_at");
CREATE INDEX IF NOT EXISTS "idx_qase_results_member_id" ON "qase_results" ("member_id");
CREATE INDEX IF NOT EXISTS "idx_qase_results_status" ON "qase_results" ("status");
CREATE INDEX IF NOT EXISTS "idx_qase_results_configuration_key" ON "qase_results" ("configuration_key");
CREATE INDEX IF NOT EXISTS "idx_qase_results_case_id" ON "qase_results" ("case_id");
CREATE INDEX IF NOT EXISTS "idx_qase_results_run_id" ON "qase_results" ("run_id");
CREATE UNIQUE INDEX IF NOT EXISTS "ux_result" ON "qase_results" ("project_code","result_id");
CREATE TABLE IF NOT EXISTS "qase_defects" ("id" text,"project_code" text,"defect_id" bigint,"title" text,"status" text,"environment" text,"jira_key" text,"jira_assignee" text,"jira_reporter" text,"jira_creator" text,"jira_priority" text,"jira_status" text,"jira_created_at" timestamptz,"qase_created_at" timestamptz,"updated_at" timestamptz,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX IF NOT EXISTS "ux_defect" ON "qase_defects" ("project_code","defect_id");
CREATE TABLE IF NOT EXISTS "sync_jobs" ("id" text,"request_key" text,"trigger" text,"status" text,"project_id" text,"sources" text,"requested_at" timestamptz,"started_at" timestamptz,"finished_at" timestamptz,"heartbeat_at" timestamptz,"attempts" bigint,"actor" text,PRIMARY KEY ("id"));
CREATE INDEX IF NOT EXISTS "idx_sync_jobs_requested_at" ON "sync_jobs" ("requested_at");
CREATE INDEX IF NOT EXISTS "idx_sync_jobs_project_id" ON "sync_jobs" ("project_id");
CREATE INDEX IF NOT EXISTS "idx_sync_jobs_status" ON "sync_jobs" ("status");
CREATE UNIQUE INDEX IF NOT EXISTS "idx_sync_jobs_request_key" ON "sync_jobs" ("request_key");
CREATE TABLE IF NOT EXISTS "sync_steps" ("id" text,"job_id" text,"source" text,"status" text,"fetched" bigint,"inserted" bigint,"updated" bigint,"skipped" bigint,"error_code" text,"started_at" timestamptz,"finished_at" timestamptz,PRIMARY KEY ("id"));
CREATE INDEX IF NOT EXISTS "idx_sync_steps_job_id" ON "sync_steps" ("job_id");
CREATE TABLE IF NOT EXISTS "sync_events" ("id" text,"job_id" text,"step_id" text,"occurred_at" timestamptz,"level" text,"code" text,"message" text,PRIMARY KEY ("id"));
CREATE INDEX IF NOT EXISTS "idx_sync_events_occurred_at" ON "sync_events" ("occurred_at");
CREATE INDEX IF NOT EXISTS "idx_sync_events_job_id" ON "sync_events" ("job_id");
CREATE TABLE IF NOT EXISTS "sync_cursors" ("id" text,"source" text,"scope_key" text,"resource" text,"watermark" timestamptz,"updated_at" timestamptz,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX IF NOT EXISTS "ux_cursor" ON "sync_cursors" ("source","scope_key","resource");

COMMIT;
