package monitoring

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/repository"
	"github.com/labstack/echo/v4"
)

var documentTypes = map[string]bool{
	"Shift Left Testing": true, "Test Plan": true, "Test Cases": true,
	"Documentation": true, "User Guideline": true, "Other": true,
}

var documentStatuses = map[string]bool{
	"Draft": true, "In Review": true, "Approved": true, "Stalled": true,
}

type documentRequest struct {
	ProjectID    string `json:"projectId"`
	DocumentType string `json:"documentType"`
	Title        string `json:"title"`
	DirectURL    string `json:"directUrl"`
	OwnerID      string `json:"ownerId"`
	Status       string `json:"status"`
}

type documentPatch struct {
	ProjectID    *string `json:"projectId"`
	DocumentType *string `json:"documentType"`
	Title        *string `json:"title"`
	DirectURL    *string `json:"directUrl"`
	OwnerID      *string `json:"ownerId"`
	Status       *string `json:"status"`
}

func (a API) RegisterDocumentationRoutes(group *echo.Group) {
	group.GET("/documents", a.documents)
	group.POST("/documents", a.createDocument, a.managerAuth)
	group.PATCH("/documents/:id", a.updateDocument, a.managerAuth)
	group.DELETE("/documents/:id", a.deleteDocument, a.managerAuth)
}

func documentPage(c echo.Context) (int, int) {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("pageSize"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return page, pageSize
}

func documentURL(value string) bool {
	u, err := url.ParseRequestURI(value)
	return err == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https")
}

func normalizeDocument(req *documentRequest) {
	req.ProjectID = strings.TrimSpace(req.ProjectID)
	req.DocumentType = strings.TrimSpace(req.DocumentType)
	req.Title = strings.TrimSpace(req.Title)
	req.DirectURL = strings.TrimSpace(req.DirectURL)
	req.OwnerID = strings.TrimSpace(req.OwnerID)
	req.Status = strings.TrimSpace(req.Status)
}

func validDocument(req documentRequest) bool {
	return req.ProjectID != "" && req.Title != "" && req.OwnerID != "" && documentTypes[req.DocumentType] && documentStatuses[req.Status] && documentURL(req.DirectURL)
}

func invalidDocument(c echo.Context) error {
	return c.JSON(http.StatusBadRequest, map[string]string{"code": "INVALID_DOCUMENT", "message": "projectId, title, active owner, allowed documentType/status, and an http/https directUrl are required"})
}

func (a API) documents(c echo.Context) error {
	page, pageSize := documentPage(c)
	filter := repository.QADocumentFilter{
		ProjectID: strings.TrimSpace(c.QueryParam("projectId")), DocumentType: strings.TrimSpace(c.QueryParam("documentType")),
		OwnerID: strings.TrimSpace(c.QueryParam("ownerId")), Status: strings.TrimSpace(c.QueryParam("status")),
		Search: c.QueryParam("q"), Page: page, PageSize: pageSize,
	}
	if filter.DocumentType != "" && !documentTypes[filter.DocumentType] || filter.Status != "" && !documentStatuses[filter.Status] {
		return invalidDocument(c)
	}
	result, err := a.Repo.Documents(filter)
	if err != nil {
		return safeError(c, err)
	}
	return a.send(c, result)
}

func (a API) validateDocumentReferences(c echo.Context, projectID, ownerID string) error {
	projectExists, ownerActive, err := a.Repo.DocumentReferences(projectID, ownerID)
	if err != nil {
		return safeError(c, err)
	}
	if !projectExists {
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"code": "PROJECT_NOT_FOUND", "message": "projectId does not reference a project"})
	}
	if !ownerActive {
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"code": "OWNER_NOT_ACTIVE", "message": "ownerId must reference an active QA member"})
	}
	return nil
}

func (a API) createDocument(c echo.Context) error {
	var req documentRequest
	if err := c.Bind(&req); err != nil {
		return invalidDocument(c)
	}
	normalizeDocument(&req)
	if !validDocument(req) {
		return invalidDocument(c)
	}
	if err := a.validateDocumentReferences(c, req.ProjectID, req.OwnerID); err != nil {
		return err
	}
	document := repository.QADocument{ProjectID: req.ProjectID, DocumentType: req.DocumentType, Title: req.Title, DirectURL: req.DirectURL, OwnerMemberID: req.OwnerID, Status: req.Status}
	if err := a.Repo.CreateDocument(&document); err != nil {
		return safeError(c, err)
	}
	return c.JSON(http.StatusCreated, document)
}

func (a API) updateDocument(c echo.Context) error {
	current, err := a.Repo.Document(c.Param("id"))
	if errors.Is(err, repository.ErrNotFound) {
		return c.JSON(http.StatusNotFound, map[string]string{"code": "NOT_FOUND", "message": "document not found"})
	}
	if err != nil {
		return safeError(c, err)
	}
	var patch documentPatch
	if err := c.Bind(&patch); err != nil {
		return invalidDocument(c)
	}
	req := documentRequest{ProjectID: current.ProjectID, DocumentType: current.DocumentType, Title: current.Title, DirectURL: current.DirectURL, OwnerID: current.OwnerMemberID, Status: current.Status}
	changes := map[string]any{}
	if patch.ProjectID != nil {
		req.ProjectID = *patch.ProjectID
		changes["project_id"] = strings.TrimSpace(*patch.ProjectID)
	}
	if patch.DocumentType != nil {
		req.DocumentType = *patch.DocumentType
		changes["document_type"] = strings.TrimSpace(*patch.DocumentType)
	}
	if patch.Title != nil {
		req.Title = *patch.Title
		changes["title"] = strings.TrimSpace(*patch.Title)
	}
	if patch.DirectURL != nil {
		req.DirectURL = *patch.DirectURL
		changes["direct_url"] = strings.TrimSpace(*patch.DirectURL)
	}
	if patch.OwnerID != nil {
		req.OwnerID = *patch.OwnerID
		changes["owner_member_id"] = strings.TrimSpace(*patch.OwnerID)
	}
	if patch.Status != nil {
		req.Status = *patch.Status
		changes["status"] = strings.TrimSpace(*patch.Status)
	}
	if len(changes) == 0 {
		return invalidDocument(c)
	}
	normalizeDocument(&req)
	if !validDocument(req) {
		return invalidDocument(c)
	}
	if err := a.validateDocumentReferences(c, req.ProjectID, req.OwnerID); err != nil {
		return err
	}
	changes["updated_at"] = time.Now().UTC()
	updated, err := a.Repo.UpdateDocument(current.ID, changes)
	if err != nil {
		return safeError(c, err)
	}
	return c.JSON(http.StatusOK, updated)
}

func (a API) deleteDocument(c echo.Context) error {
	err := a.Repo.DeleteDocument(c.Param("id"))
	if errors.Is(err, repository.ErrNotFound) {
		return c.JSON(http.StatusNotFound, map[string]string{"code": "NOT_FOUND", "message": "document not found"})
	}
	if err != nil {
		return safeError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
