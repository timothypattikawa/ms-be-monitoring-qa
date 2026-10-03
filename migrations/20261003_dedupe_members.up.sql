-- Merge duplicate members (same name, case/space-insensitive) into one row.
-- Survivor per name: active first, then has qase name, then has email, then lowest id.
-- Survivor gets the non-empty jira_email / jira_account_id / qase_member_id of
-- its duplicates, the largest weekly_capacity_hours, and active if any was active.
-- FK references (qa_documents, project_qa_assignments) are repointed before the
-- duplicates are deleted. Idempotent: a second run finds nothing to merge.
BEGIN;

CREATE TEMP TABLE member_dedupe ON COMMIT DROP AS
SELECT id,
       first_value(id) OVER w AS keep_id
FROM members
WINDOW w AS (
    PARTITION BY lower(btrim(name))
    ORDER BY active DESC,
             (coalesce(qase_member_id, '') <> '') DESC,
             (coalesce(jira_email, '') <> '') DESC,
             id
);
DELETE FROM member_dedupe WHERE id = keep_id;

UPDATE members k SET
    jira_email = coalesce(nullif(k.jira_email, ''), (SELECT nullif(m.jira_email, '') FROM members m JOIN member_dedupe d ON d.id = m.id WHERE d.keep_id = k.id AND nullif(m.jira_email, '') IS NOT NULL ORDER BY m.id LIMIT 1), k.jira_email),
    jira_account_id = coalesce(nullif(k.jira_account_id, ''), (SELECT nullif(m.jira_account_id, '') FROM members m JOIN member_dedupe d ON d.id = m.id WHERE d.keep_id = k.id AND nullif(m.jira_account_id, '') IS NOT NULL ORDER BY m.id LIMIT 1), k.jira_account_id),
    qase_member_id = coalesce(nullif(k.qase_member_id, ''), (SELECT nullif(m.qase_member_id, '') FROM members m JOIN member_dedupe d ON d.id = m.id WHERE d.keep_id = k.id AND nullif(m.qase_member_id, '') IS NOT NULL ORDER BY m.id LIMIT 1), k.qase_member_id),
    weekly_capacity_hours = greatest(k.weekly_capacity_hours, coalesce((SELECT max(m.weekly_capacity_hours) FROM members m JOIN member_dedupe d ON d.id = m.id WHERE d.keep_id = k.id), 0)),
    active = k.active OR coalesce((SELECT bool_or(m.active) FROM members m JOIN member_dedupe d ON d.id = m.id WHERE d.keep_id = k.id), false)
WHERE k.id IN (SELECT keep_id FROM member_dedupe);

UPDATE qa_documents SET owner_member_id = d.keep_id FROM member_dedupe d WHERE qa_documents.owner_member_id = d.id;
UPDATE project_qa_assignments a SET member_id = d.keep_id FROM member_dedupe d WHERE a.member_id = d.id;

DELETE FROM members WHERE id IN (SELECT id FROM member_dedupe);

COMMIT;
