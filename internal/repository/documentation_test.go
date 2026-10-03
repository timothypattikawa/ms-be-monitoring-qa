package repository

import (
	"testing"
	"time"
)

func TestDocumentCRUDListAndSoftDelete(t *testing.T) {
	repo := NewSQLiteForTest(t)
	if err := repo.AutoMigrateDocumentation(); err != nil {
		t.Fatal(err)
	}
	project := Project{ID: "project-1", JiraInitKey: "INIT-1", Name: "Checkout", Status: "active"}
	member := Member{ID: "member-1", Name: "Nadia", Active: true}
	if err := repo.SaveProject(&project); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveMember(&member); err != nil {
		t.Fatal(err)
	}

	document := QADocument{ProjectID: project.ID, OwnerMemberID: member.ID, DocumentType: "Test Plan", Title: "Checkout plan", DirectURL: "https://example.com/plan", Status: "Draft"}
	if err := repo.CreateDocument(&document); err != nil {
		t.Fatal(err)
	}
	page, err := repo.Documents(QADocumentFilter{ProjectID: project.ID, Status: "Draft", Search: "checkout", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ProjectName != project.Name || page.Items[0].OwnerName != member.Name {
		t.Fatalf("unexpected page: %+v", page)
	}
	updated, err := repo.UpdateDocument(document.ID, map[string]any{"status": "Approved", "updated_at": time.Now().UTC()})
	if err != nil || updated.Status != "Approved" {
		t.Fatalf("update failed: %+v, %v", updated, err)
	}
	if err := repo.DeleteDocument(document.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Document(document.ID); !IsDocumentNotFound(err) {
		t.Fatalf("soft-deleted document is still visible: %v", err)
	}
	var deleted int64
	if err := repo.db.Unscoped().Model(&QADocument{}).Where("id = ? AND deleted_at IS NOT NULL", document.ID).Count(&deleted).Error; err != nil || deleted != 1 {
		t.Fatalf("document was not soft deleted: count=%d err=%v", deleted, err)
	}
}

func TestDocumentReferencesRequireActiveMember(t *testing.T) {
	repo := NewSQLiteForTest(t)
	if err := repo.SaveProject(&Project{ID: "project-1", JiraInitKey: "INIT-1", Name: "Checkout"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveMember(&Member{ID: "member-1", Name: "Inactive", Active: false}); err != nil {
		t.Fatal(err)
	}
	projectExists, ownerActive, err := repo.DocumentReferences("project-1", "member-1")
	if err != nil || !projectExists || ownerActive {
		t.Fatalf("unexpected references: project=%v owner=%v err=%v", projectExists, ownerActive, err)
	}
}
