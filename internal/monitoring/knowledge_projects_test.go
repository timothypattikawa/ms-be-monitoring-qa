package monitoring

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func projectsSolr(t *testing.T, statusFail bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/admin/cores":
			if statusFail {
				http.Error(w, "boom", 500)
				return
			}
			w.Write([]byte(`{"status":{"testcase_vectors":{"index":{"numDocs":10}},"other":{"index":{"numDocs":5}},
			 "tc_promo":{"index":{"numDocs":8}},"tc_apomitra":{"index":{"numDocs":3}},"tc_empty":{"index":{"numDocs":0}},
			 "tc_broken":{"index":{"numDocs":7}},"tc_new_thing":{"index":{"numDocs":1}}}}`))
		case "/testcase_vectors/select":
			w.Write([]byte(`{"facet_counts":{"facet_fields":{"project":["B",4,"A",4,"C",2]}}}`))
		case "/tc_promo/select":
			w.Write([]byte(`{"facet_counts":{"facet_fields":{"project":["A",5," INIT434 ",3]}}}`))
		case "/tc_apomitra/select":
			w.Write([]byte(`{"facet_counts":{"facet_fields":{"project":["Z",3]}}}`))
		case "/tc_new_thing/select":
			w.Write([]byte(`{"facet_counts":{"facet_fields":{"project":[]}}}`))
		default: // tc_broken and anything else
			http.Error(w, "boom", 500)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestKnowledgeProjects(t *testing.T) {
	type resp struct {
		Totals struct {
			Collections, Docs, Projects, EmptyCollections int
		}
		Collections []struct {
			Name, Label, Error string
			DocCount           int64
			ProjectCount       int
			Projects           []struct {
				Code, Name string
				Docs       int64
			}
		}
	}
	srv := projectsSolr(t, false)
	api, e := knowledgeAPI(t, srv.URL, "", nil)
	// name fallback: local projects table (no Qase token configured)
	if _, err := api.Repo.SaveProjectMapping("INIT-434", "INIT-434 Promo 522", "INIT434", "qa", time.Now(), time.Now(), time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	rec := do(e, http.MethodGet, "/api/v1/knowledge/projects")
	if rec.Code != 200 {
		t.Fatalf("code %d %s", rec.Code, rec.Body)
	}
	var got resp
	data(t, rec, &got)

	var names []string
	for _, c := range got.Collections {
		names = append(names, c.Name)
	}
	if strings.Join(names, ",") != "testcase_vectors,tc_apomitra,tc_broken,tc_empty,tc_new_thing,tc_promo" {
		t.Fatalf("order: %v", names)
	}
	checks := []struct {
		name, label, err string
		docs             int64
		projects         string
	}{
		{"testcase_vectors", "Default / Utama", "", 10, "A:4,B:4,C:2"}, // docs desc, then code
		{"tc_apomitra", "APO Mitra", "", 3, "Z:3"},
		{"tc_broken", "Broken", "facet_failed", 7, ""}, // one core failing keeps docCount
		{"tc_empty", "Empty", "", 0, ""},
		{"tc_new_thing", "New Thing", "", 1, ""},
		{"tc_promo", "Promo", "", 8, "A:5,INIT434:3"}, // value trimmed
	}
	for i, w := range checks {
		c := got.Collections[i]
		var ps []string
		for _, p := range c.Projects {
			ps = append(ps, p.Code+":"+strconv.FormatInt(p.Docs, 10))
		}
		if c.Label != w.label || c.Error != w.err || c.DocCount != w.docs || strings.Join(ps, ",") != w.projects || c.ProjectCount != len(c.Projects) {
			t.Errorf("%s: %+v want %+v", w.name, c, w)
		}
	}
	// distinct projects: A,B,C,Z,INIT434; empty collections: tc_empty only
	if got.Totals.Collections != 6 || got.Totals.Docs != 29 || got.Totals.Projects != 5 || got.Totals.EmptyCollections != 1 {
		t.Errorf("totals %+v", got.Totals)
	}
	if n := got.Collections[5].Projects[1].Name; n != "INIT-434 Promo 522" {
		t.Errorf("fallback name %q", n)
	}
	if n := got.Collections[0].Projects[0].Name; n != "" {
		t.Errorf("unknown name should be empty, got %q", n)
	}
	if !strings.Contains(rec.Body.String(), `"projects":[]`) {
		t.Errorf("empty projects must serialize as []: %s", rec.Body)
	}
}

func TestKnowledgeProjectsQaseNamesAndSolrDown(t *testing.T) {
	qase := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/project" || r.Header.Get("Token") != "tok" {
			http.Error(w, "no", 403)
			return
		}
		w.Write([]byte(`{"status":true,"result":{"total":2,"entities":[{"code":"A","title":"Alpha"},{"code":"B","title":"Beta"}]}}`))
	}))
	defer qase.Close()
	srv := projectsSolr(t, false)
	api, e := knowledgeAPI(t, srv.URL, "", nil)
	_ = api
	api.Connector = Connector{Client: qase.Client(), QaseBaseURL: qase.URL, QaseToken: "tok"}
	e2 := echo.New()
	api.RegisterKnowledgeRoutes(e2.Group("/api/v1"))
	rec := do(e2, http.MethodGet, "/api/v1/knowledge/projects")
	if !strings.Contains(rec.Body.String(), `"code":"A","name":"Alpha"`) {
		t.Errorf("qase title missing: %s", rec.Body)
	}
	_ = e

	down := projectsSolr(t, true)
	_, e3 := knowledgeAPI(t, down.URL, "", nil)
	rec = do(e3, http.MethodGet, "/api/v1/knowledge/projects")
	if rec.Code != 502 || !strings.Contains(rec.Body.String(), "SOLR_UNAVAILABLE") || strings.Contains(rec.Body.String(), down.URL) {
		t.Errorf("status failure: %d %s", rec.Code, rec.Body)
	}
}
