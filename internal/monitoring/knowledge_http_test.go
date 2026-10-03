package monitoring

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/repository"
	"github.com/labstack/echo/v4"
)

func fakeSolr(t *testing.T, selectQ *string) *httptest.Server {
	t.Helper()
	old := time.Now().Add(-30 * 24 * time.Hour).UTC().Format(time.RFC3339)
	fresh := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/admin/cores":
			w.Write([]byte(`{"status":{
			 "testcase_vectors":{"index":{"numDocs":99,"sizeInBytes":1,"lastModified":"` + fresh + `"}},
			 "other":{"index":{"numDocs":5,"sizeInBytes":1,"lastModified":"` + fresh + `"}},
			 "tc_ok":{"index":{"numDocs":95,"sizeInBytes":10,"lastModified":"` + fresh + `"}},
			 "tc_stale":{"index":{"numDocs":50,"sizeInBytes":20,"lastModified":"` + old + `"}},
			 "tc_empty":{"index":{"numDocs":0,"sizeInBytes":0,"lastModified":"` + fresh + `"}},
			 "tc_low":{"index":{"numDocs":50,"sizeInBytes":5,"lastModified":"` + fresh + `"}}}}`))
		case r.URL.Path == "/admin/info/system":
			w.Write([]byte(`{"jvm":{"memory":{"raw":{"used":1073741824,"max":4294967296}}}}`))
		case strings.HasSuffix(r.URL.Path, "/schema/fieldtypes"):
			w.Write([]byte(`{"fieldTypes":[{"class":"solr.StrField"},{"class":"solr.DenseVectorField","vectorDimension":768}]}`))
		case strings.HasSuffix(r.URL.Path, "/select"):
			if selectQ != nil {
				*selectQ = r.URL.Path + "|" + r.URL.Query().Get("q")
			}
			w.Write([]byte(`{"response":{"numFound":1,"docs":[{"id":"RM-36","title":["Login ok","x"],"source":"https://qase.io/RM-36","project":"OK"}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func knowledgeAPI(t *testing.T, solrURL, webhook string, targets map[string]int64) (API, *echo.Echo) {
	t.Helper()
	api := API{Repo: repository.NewSQLiteForTest(t), Solr: Solr{BaseURL: solrURL, VectorCollection: "testcase_vectors", Prefix: "tc_",
		Model: "m", Dimension: 1536, Targets: targets, StaleAfter: 14 * 24 * time.Hour, WebhookURL: webhook, AutoSyncEvery: "15m"}}
	e := echo.New()
	api.RegisterKnowledgeRoutes(e.Group("/api/v1"))
	return api, e
}

func do(e *echo.Echo, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func data(t *testing.T, rec *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("bad body %d: %s", rec.Code, rec.Body.String())
	}
}

func TestKnowledgeCollectionsStatusAndFiltering(t *testing.T) {
	srv := fakeSolr(t, nil)
	_, e := knowledgeAPI(t, srv.URL, "", map[string]int64{"tc_ok": 100, "tc_low": 100, "tc_stale": 50})
	rec := do(e, http.MethodGet, "/api/v1/knowledge/collections")
	var got []knowledgeCollection
	data(t, rec, &got)
	want := map[string]string{"tc_empty": "NEEDS_REINDEX", "tc_low": "NEEDS_REINDEX", "tc_ok": "HEALTHY", "tc_stale": "OUTDATED_SYNC"}
	if len(got) != len(want) {
		t.Fatalf("expected %d collections (no testcase_vectors/other), got %+v", len(want), got)
	}
	for _, c := range got {
		if want[c.Name] != c.Status {
			t.Errorf("%s: want %s got %s", c.Name, want[c.Name], c.Status)
		}
		if c.Name == "tc_empty" && (c.CoveragePct != nil || c.Target != nil) {
			t.Errorf("coverage must be null without target")
		}
		if c.Name == "tc_ok" && (c.CoveragePct == nil || *c.CoveragePct != 95) {
			t.Errorf("tc_ok coverage: %+v", c.CoveragePct)
		}
	}
}

func TestKnowledgeOverview(t *testing.T) {
	srv := fakeSolr(t, nil)
	_, e := knowledgeAPI(t, srv.URL, "", nil)
	var got knowledgeOverview
	data(t, do(e, http.MethodGet, "/api/v1/knowledge/overview"), &got)
	if got.RAM.UsedGb != 1 || got.RAM.TotalGb != 4 || got.RAM.Pct != 25 || got.Embedding.Dimension != 768 || got.CollectionsActive != 3 || got.LastFullSync == nil || got.AutoSyncEvery != "15m" {
		t.Fatalf("%+v", got)
	}
}

func TestKnowledgeDocuments(t *testing.T) {
	var q string
	srv := fakeSolr(t, &q)
	_, e := knowledgeAPI(t, srv.URL, "", nil)
	var got struct {
		Items []knowledgeDocument
		Total int64
	}
	data(t, do(e, http.MethodGet, "/api/v1/knowledge/documents?collection=tc_ok&q=a:b+c"), &got)
	if q != "/tc_ok/select|title:*a\\:b\\ c* OR id:*a\\:b\\ c*" {
		t.Fatalf("query: %s", q)
	}
	d := got.Items[0]
	if got.Total != 1 || d.Title != "Login ok" || d.Key != "RM-36" || d.SourceURL != "https://qase.io/RM-36" || d.Collection != "tc_ok" || d.Chunks != 1 || d.Dims != 768 || d.SyncStatus != "SYNCED" || d.LastSyncedAt == nil {
		t.Fatalf("%+v", d)
	}
	// no collection => vector collection, doc mapped to its project core
	data(t, do(e, http.MethodGet, "/api/v1/knowledge/documents"), &got)
	if q != "/testcase_vectors/select|*:*" || got.Items[0].Collection != "tc_ok" {
		t.Fatalf("%s %+v", q, got.Items[0])
	}
}

func TestKnowledgeCollectionValidation(t *testing.T) {
	srv := fakeSolr(t, nil)
	_, e := knowledgeAPI(t, srv.URL, "http://unused", nil)
	for _, path := range []string{"/api/v1/knowledge/documents?collection=../admin", "/api/v1/knowledge/documents?collection=testcase_vectors", "/api/v1/knowledge/documents?collection=other"} {
		if rec := do(e, http.MethodGet, path); rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d", path, rec.Code)
		}
	}
	if rec := do(e, http.MethodPost, "/api/v1/knowledge/collections/nope/sync"); rec.Code != http.StatusNotFound {
		t.Errorf("sync unknown: %d", rec.Code)
	}
}

func TestKnowledgeSolrDownDoesNotLeak(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	_, e := knowledgeAPI(t, url, "", nil)
	rec := do(e, http.MethodGet, "/api/v1/knowledge/collections")
	if rec.Code != http.StatusBadGateway || strings.Contains(rec.Body.String(), "127.0.0.1") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestKnowledgeTriggerWebhook(t *testing.T) {
	srv := fakeSolr(t, nil)
	var body map[string]string
	status := http.StatusOK
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(status)
	}))
	defer hook.Close()
	tests := []struct {
		name, webhook, path string
		hookStatus, want    int
	}{
		{"not configured", "", "sync", 200, 501},
		{"accepted", hook.URL, "reindex", 200, 202},
		{"pipeline fails", hook.URL, "sync", 500, 502},
		{"unreachable", "http://127.0.0.1:1", "sync", 200, 502},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status = tt.hookStatus
			_, e := knowledgeAPI(t, srv.URL, tt.webhook, nil)
			if rec := do(e, http.MethodPost, "/api/v1/knowledge/collections/tc_ok/"+tt.path); rec.Code != tt.want {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
		})
	}
	if body["collection"] != "tc_ok" || body["mode"] != "sync" {
		t.Fatalf("webhook body %v", body)
	}
}

func TestParseStaleAfter(t *testing.T) {
	for in, want := range map[string]time.Duration{"14d": 14 * 24 * time.Hour, "1.5d": 36 * time.Hour, "48h": 48 * time.Hour, "": 14 * 24 * time.Hour, "junk": 14 * 24 * time.Hour, "-2d": 14 * 24 * time.Hour} {
		if got := parseStaleAfter(in); got != want {
			t.Errorf("%q: %v", in, got)
		}
	}
}

func TestParseTargets(t *testing.T) {
	got := parseTargets(" tc_a=10, tc_b = 20,bad,tc_c=x,")
	if len(got) != 2 || got["tc_a"] != 10 || got["tc_b"] != 20 {
		t.Fatalf("%v", got)
	}
}
