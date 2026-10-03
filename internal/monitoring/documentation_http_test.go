package monitoring

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/repository"
	"github.com/labstack/echo/v4"
)

func documentationAPI(t *testing.T) (API, *echo.Echo) {
	t.Helper()
	repo := repository.NewSQLiteForTest(t)
	if err := repo.AutoMigrateDocumentation(); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveProject(&repository.Project{ID: "project-1", JiraInitKey: "INIT-1", Name: "Checkout", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveMember(&repository.Member{ID: "member-1", Name: "Nadia", Active: true}); err != nil {
		t.Fatal(err)
	}
	api := API{Repo: repo}
	e := echo.New()
	api.RegisterDocumentationRoutes(e.Group("/api/v1"))
	return api, e
}

func TestDocumentationRejectsNonHTTPURL(t *testing.T) {
	_, e := documentationAPI(t)
	body := `{"projectId":"project-1","documentType":"Test Plan","title":"Plan","directUrl":"javascript:alert(1)","ownerId":"member-1","status":"Draft"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDocumentationCRUDAndPagination(t *testing.T) {
	_, e := documentationAPI(t)
	body := `{"projectId":"project-1","documentType":"Test Plan","title":" Checkout plan ","directUrl":"https://example.com/plan","ownerId":"member-1","status":"Draft"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created repository.QADocument
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	patch := `{"status":"Approved"}`
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/documents/"+created.ID, strings.NewReader(patch))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"Approved"`) {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/documents?status=Approved&q=checkout&page=1&pageSize=1", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"total":1`) || !strings.Contains(rec.Body.String(), `"ownerName":"Nadia"`) {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/documents/"+created.ID, nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
}

func TestDocumentationMutationsUseManagerAuth(t *testing.T) {
	t.Setenv("MONITOR_MANAGER_API_KEY", "secret")
	_, e := documentationAPI(t)
	body := `{"projectId":"project-1","documentType":"Test Plan","title":"Plan","directUrl":"https://example.com/plan","ownerId":"member-1","status":"Draft"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected manager auth rejection, got %d: %s", rec.Code, rec.Body.String())
	}
}
