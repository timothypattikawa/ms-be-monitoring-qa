package repository

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type QADocument struct {
	ID            string         `gorm:"primaryKey" json:"id"`
	ProjectID     string         `gorm:"index" json:"projectId"`
	DocumentType  string         `json:"documentType"`
	Title         string         `json:"title"`
	DirectURL     string         `json:"directUrl"`
	OwnerMemberID string         `gorm:"index" json:"ownerId"`
	Status        string         `gorm:"index" json:"status"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

type QADocumentView struct {
	QADocument
	ProjectName string `json:"projectName"`
	OwnerName   string `json:"ownerName"`
}

type QADocumentFilter struct {
	ProjectID    string
	DocumentType string
	OwnerID      string
	Status       string
	Search       string
	Page         int
	PageSize     int
}

type QADocumentPage struct {
	Items    []QADocumentView `json:"items"`
	Page     int              `json:"page"`
	PageSize int              `json:"pageSize"`
	Total    int64            `json:"total"`
}

func (r *Monitoring) AutoMigrateDocumentation() error { return r.db.AutoMigrate(&QADocument{}) }

func (r *Monitoring) DocumentReferences(projectID, ownerID string) (projectExists, ownerActive bool, err error) {
	var projects int64
	if err = r.db.Model(&Project{}).Where("id = ?", projectID).Count(&projects).Error; err != nil {
		return false, false, err
	}
	var owners int64
	if err = r.db.Model(&Member{}).Where("id = ? AND active = ?", ownerID, true).Count(&owners).Error; err != nil {
		return projects > 0, false, err
	}
	return projects > 0, owners > 0, nil
}

func (r *Monitoring) Documents(filter QADocumentFilter) (QADocumentPage, error) {
	out := QADocumentPage{Items: make([]QADocumentView, 0), Page: filter.Page, PageSize: filter.PageSize}
	q := r.db.Model(&QADocument{}).
		Joins("JOIN projects ON projects.id = qa_documents.project_id").
		Joins("JOIN members ON members.id = qa_documents.owner_member_id")
	if filter.ProjectID != "" {
		q = q.Where("qa_documents.project_id = ?", filter.ProjectID)
	}
	if filter.DocumentType != "" {
		q = q.Where("qa_documents.document_type = ?", filter.DocumentType)
	}
	if filter.OwnerID != "" {
		q = q.Where("qa_documents.owner_member_id = ?", filter.OwnerID)
	}
	if filter.Status != "" {
		q = q.Where("qa_documents.status = ?", filter.Status)
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		q = q.Where("LOWER(qa_documents.title) LIKE ?", "%"+strings.ToLower(search)+"%")
	}
	if err := q.Count(&out.Total).Error; err != nil {
		return out, err
	}
	err := q.Select("qa_documents.*, projects.name AS project_name, members.name AS owner_name").
		Order("qa_documents.updated_at DESC, qa_documents.id ASC").
		Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Scan(&out.Items).Error
	return out, err
}

func (r *Monitoring) Document(id string) (QADocument, error) {
	var out QADocument
	err := r.db.First(&out, "id = ?", id).Error
	return out, err
}

func (r *Monitoring) CreateDocument(document *QADocument) error {
	if document.ID == "" {
		document.ID = uuid.NewString()
	}
	return r.db.Create(document).Error
}

func (r *Monitoring) UpdateDocument(id string, changes map[string]any) (QADocument, error) {
	result := r.db.Model(&QADocument{}).Where("id = ?", id).Updates(changes)
	if result.Error != nil {
		return QADocument{}, result.Error
	}
	if result.RowsAffected == 0 {
		return QADocument{}, ErrNotFound
	}
	return r.Document(id)
}

func (r *Monitoring) DeleteDocument(id string) error {
	result := r.db.Delete(&QADocument{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func IsDocumentNotFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }
