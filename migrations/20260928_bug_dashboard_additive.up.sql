BEGIN;

ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS project_size varchar(8),
    ADD COLUMN IF NOT EXISTS staging_mttt_minutes integer,
    ADD COLUMN IF NOT EXISTS beta_mttt_minutes integer;

ALTER TABLE jira_issues
    ADD COLUMN IF NOT EXISTS parent_key varchar,
    ADD COLUMN IF NOT EXISTS environment varchar,
    ADD COLUMN IF NOT EXISTS created_at timestamptz,
    ADD COLUMN IF NOT EXISTS active boolean NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS sync_scope varchar;

UPDATE jira_issues SET active = true WHERE active IS NULL;

CREATE TABLE IF NOT EXISTS project_qa_assignments (
    project_id text NOT NULL REFERENCES projects(id),
    jira_account_id varchar NOT NULL,
    member_id text REFERENCES members(id),
    active boolean NOT NULL DEFAULT true,
    synced_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, jira_account_id)
);

CREATE TABLE IF NOT EXISTS qa_documents (
    id text PRIMARY KEY,
    project_id text NOT NULL REFERENCES projects(id),
    document_type varchar NOT NULL,
    title varchar NOT NULL,
    direct_url text NOT NULL,
    owner_member_id text NOT NULL REFERENCES members(id),
    status varchar NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

DO $$ BEGIN
    ALTER TABLE projects ADD CONSTRAINT chk_projects_project_size
        CHECK (project_size IS NULL OR project_size IN ('S', 'M', 'L', 'XL', '2XL', '3XL', '4L', '5L')) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TABLE projects ADD CONSTRAINT chk_projects_staging_mttt
        CHECK (staging_mttt_minutes IS NULL OR staging_mttt_minutes >= 0) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TABLE projects ADD CONSTRAINT chk_projects_beta_mttt
        CHECK (beta_mttt_minutes IS NULL OR beta_mttt_minutes >= 0) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TABLE qa_documents ADD CONSTRAINT chk_qa_documents_type
        CHECK (document_type IN ('Test Strategy', 'Test Plan', 'Test Cases', 'SOP', 'Release Notes', 'Other')) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TABLE qa_documents ADD CONSTRAINT chk_qa_documents_status
        CHECK (status IN ('Draft', 'In Review', 'Approved', 'Stalled')) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TABLE qa_documents ADD CONSTRAINT chk_qa_documents_direct_url
        CHECK (direct_url ~* '^https?://') NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE INDEX IF NOT EXISTS idx_qase_results_project_run_ended ON qase_results(project_code, run_id, ended_at);
CREATE INDEX IF NOT EXISTS idx_qase_results_project_tester_ended ON qase_results(project_code, tester_name, ended_at);
CREATE INDEX IF NOT EXISTS idx_qase_run_cases_project_run_case ON qase_run_cases(project_code, run_id, case_id);
CREATE INDEX IF NOT EXISTS idx_qase_cases_project_case ON qase_cases(project_code, case_id);
CREATE INDEX IF NOT EXISTS idx_jira_issues_project_active_environment_created ON jira_issues(project_id, active, environment, created_at);
CREATE INDEX IF NOT EXISTS idx_jira_issues_parent_active ON jira_issues(parent_key, active);
CREATE INDEX IF NOT EXISTS idx_project_qa_assignments_member_active ON project_qa_assignments(member_id, active);
CREATE INDEX IF NOT EXISTS idx_qa_documents_project_status_updated ON qa_documents(project_id, status, updated_at) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_qa_documents_owner_status ON qa_documents(owner_member_id, status) WHERE deleted_at IS NULL;

COMMIT;
