CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS projects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    jira_init_id text NOT NULL UNIQUE,
    jira_init_key text NOT NULL,
    name text NOT NULL,
    status text NOT NULL,
    health text NOT NULL,
    qa_owner text NOT NULL,
    qase_project_code text NOT NULL,
    staging_date timestamptz,
    beta_date timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_projects_jira_init_key ON projects (jira_init_key);
CREATE INDEX IF NOT EXISTS idx_projects_qase_project_code ON projects (qase_project_code);

CREATE TABLE IF NOT EXISTS members (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    jira_account_id text NOT NULL,
    qase_member_id text NOT NULL,
    weekly_capacity_hours double precision NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_members_jira_account_id ON members (jira_account_id);
CREATE INDEX IF NOT EXISTS idx_members_qase_member_id ON members (qase_member_id);

CREATE TABLE IF NOT EXISTS allocations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL,
    member_id uuid NOT NULL,
    week_start timestamptz NOT NULL,
    planned_hours double precision NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_allocations_project_id ON allocations (project_id);
CREATE INDEX IF NOT EXISTS idx_allocations_member_id ON allocations (member_id);
CREATE INDEX IF NOT EXISTS idx_allocations_week_start ON allocations (week_start);

CREATE TABLE IF NOT EXISTS jira_issues (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    external_id text NOT NULL UNIQUE,
    key text NOT NULL,
    project_id uuid,
    summary text NOT NULL,
    issue_type text NOT NULL,
    severity text NOT NULL,
    status text NOT NULL,
    creator text NOT NULL,
    reporter text NOT NULL,
    assignee text NOT NULL,
    source_updated_at timestamptz,
    fetched_at timestamptz NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_jira_issues_key ON jira_issues (key);
CREATE INDEX IF NOT EXISTS idx_jira_issues_project_id ON jira_issues (project_id);
CREATE INDEX IF NOT EXISTS idx_jira_issues_issue_type ON jira_issues (issue_type);

CREATE TABLE IF NOT EXISTS qase_cases (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_code text NOT NULL,
    case_id bigint NOT NULL,
    title text NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT ux_case UNIQUE (project_code, case_id)
);

CREATE TABLE IF NOT EXISTS qase_runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_code text NOT NULL,
    run_id bigint NOT NULL,
    title text NOT NULL,
    status text NOT NULL,
    platform text NOT NULL,
    environment text NOT NULL,
    started_at timestamptz,
    finished_at timestamptz,
    fetched_at timestamptz NOT NULL,
    CONSTRAINT ux_run UNIQUE (project_code, run_id)
);

CREATE TABLE IF NOT EXISTS qase_run_cases (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_code text NOT NULL,
    run_id bigint NOT NULL,
    case_id bigint NOT NULL,
    CONSTRAINT ux_run_case UNIQUE (project_code, run_id, case_id)
);

CREATE TABLE IF NOT EXISTS qase_results (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_code text NOT NULL,
    result_id text NOT NULL,
    run_id bigint NOT NULL,
    case_id bigint NOT NULL,
    configuration_key text NOT NULL,
    status text NOT NULL,
    member_id text NOT NULL,
    environment text NOT NULL,
    platform text NOT NULL,
    started_at timestamptz,
    ended_at timestamptz,
    fetched_at timestamptz NOT NULL,
    CONSTRAINT ux_result UNIQUE (project_code, result_id)
);

CREATE INDEX IF NOT EXISTS idx_qase_results_run_id ON qase_results (run_id);
CREATE INDEX IF NOT EXISTS idx_qase_results_case_id ON qase_results (case_id);
CREATE INDEX IF NOT EXISTS idx_qase_results_configuration_key ON qase_results (configuration_key);
CREATE INDEX IF NOT EXISTS idx_qase_results_status ON qase_results (status);
CREATE INDEX IF NOT EXISTS idx_qase_results_member_id ON qase_results (member_id);
CREATE INDEX IF NOT EXISTS idx_qase_results_ended_at ON qase_results (ended_at);

CREATE TABLE IF NOT EXISTS sync_jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    request_key text NOT NULL UNIQUE,
    trigger text NOT NULL,
    status text NOT NULL,
    project_id uuid,
    sources text NOT NULL,
    requested_at timestamptz NOT NULL,
    started_at timestamptz,
    finished_at timestamptz,
    heartbeat_at timestamptz,
    attempts integer NOT NULL DEFAULT 0,
    actor text NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sync_jobs_status ON sync_jobs (status);
CREATE INDEX IF NOT EXISTS idx_sync_jobs_project_id ON sync_jobs (project_id);
CREATE INDEX IF NOT EXISTS idx_sync_jobs_requested_at ON sync_jobs (requested_at);

CREATE TABLE IF NOT EXISTS sync_steps (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id uuid NOT NULL,
    source text NOT NULL,
    status text NOT NULL,
    fetched integer NOT NULL DEFAULT 0,
    inserted integer NOT NULL DEFAULT 0,
    updated integer NOT NULL DEFAULT 0,
    skipped integer NOT NULL DEFAULT 0,
    error_code text NOT NULL DEFAULT '',
    started_at timestamptz,
    finished_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_sync_steps_job_id ON sync_steps (job_id);

CREATE TABLE IF NOT EXISTS sync_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id uuid NOT NULL,
    step_id uuid,
    occurred_at timestamptz NOT NULL,
    level text NOT NULL,
    code text NOT NULL,
    message text NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sync_events_job_id ON sync_events (job_id);
CREATE INDEX IF NOT EXISTS idx_sync_events_occurred_at ON sync_events (occurred_at);

CREATE TABLE IF NOT EXISTS sync_cursors (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source text NOT NULL,
    scope_key text NOT NULL,
    resource text NOT NULL,
    watermark timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT ux_cursor UNIQUE (source, scope_key, resource)
);
