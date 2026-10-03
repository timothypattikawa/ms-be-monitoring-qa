-- Rename document types: Test Strategy -> Shift Left Testing, SOP -> Documentation,
-- Release Notes -> User Guideline.
ALTER TABLE qa_documents DROP CONSTRAINT IF EXISTS chk_qa_documents_type;
UPDATE qa_documents SET document_type = CASE document_type
    WHEN 'Test Strategy' THEN 'Shift Left Testing'
    WHEN 'SOP' THEN 'Documentation'
    WHEN 'Release Notes' THEN 'User Guideline'
    ELSE document_type END;
ALTER TABLE qa_documents ADD CONSTRAINT chk_qa_documents_type
    CHECK (document_type IN ('Shift Left Testing', 'Test Plan', 'Test Cases', 'Documentation', 'User Guideline', 'Other')) NOT VALID;
