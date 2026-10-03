package monitoring

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

type knowledgeProject struct {
	Code string `json:"code"`
	Name string `json:"name"`
	Docs int64  `json:"docs"`
}

type knowledgeProjectCollection struct {
	Name         string             `json:"name"`
	Label        string             `json:"label"`
	DocCount     int64              `json:"docCount"`
	ProjectCount int                `json:"projectCount"`
	Projects     []knowledgeProject `json:"projects"`
	Error        string             `json:"error,omitempty"`
}

type knowledgeProjectsResponse struct {
	Totals struct {
		Collections      int   `json:"collections"`
		Docs             int64 `json:"docs"`
		Projects         int   `json:"projects"`
		EmptyCollections int   `json:"emptyCollections"`
	} `json:"totals"`
	Collections []knowledgeProjectCollection `json:"collections"`
}

var collectionLabels = map[string]string{"testcase_vectors": "Default / Utama", "tc_apo": "APO", "tc_apomitra": "APO Mitra",
	"tc_eservice": "E-Service", "tc_mylawson": "MyLawson", "tc_backoffice": "Back Office", "tc_marketplace": "Marketplace",
	"tc_webcom_waorder": "Webcom WA Order", "tc_alfagift": "Alfagift", "tc_payment": "Payment", "tc_promo": "Promo", "tc_voucher": "Voucher"}

func (s Solr) label(name string) string {
	if l, ok := collectionLabels[strings.ToLower(name)]; ok {
		return l
	}
	words := strings.Fields(strings.ReplaceAll(strings.TrimPrefix(name, s.Prefix), "_", " "))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
	}
	return strings.Join(words, " ")
}

// facetProjects returns project value -> doc count for one core via a facet on `project`.
func (s Solr) facetProjects(ctx context.Context, core string) ([]knowledgeProject, error) {
	var raw struct {
		FacetCounts struct {
			FacetFields struct {
				Project []any `json:"project"`
			} `json:"facet_fields"`
		} `json:"facet_counts"`
	}
	params := url.Values{"q": {"*:*"}, "rows": {"0"}, "facet": {"true"}, "facet.field": {"project"}, "facet.limit": {"-1"}, "facet.mincount": {"1"}}
	if err := s.get(ctx, "/"+url.PathEscape(core)+"/select", params, &raw); err != nil {
		return nil, err
	}
	flat := raw.FacetCounts.FacetFields.Project
	out := []knowledgeProject{}
	for i := 0; i+1 < len(flat); i += 2 {
		code, _ := flat[i].(string)
		n, _ := flat[i+1].(float64)
		if code = strings.TrimSpace(code); code != "" {
			out = append(out, knowledgeProject{Code: code, Docs: int64(n)})
		}
	}
	return out, nil
}

// ProjectsByCollection builds the per-collection project breakdown. names maps project code -> title.
func (s Solr) ProjectsByCollection(ctx context.Context, names func() map[string]string) (knowledgeProjectsResponse, error) {
	var res knowledgeProjectsResponse
	all, err := s.statusCores(ctx)
	if err != nil {
		return res, err
	}
	var cols []knowledgeProjectCollection
	var rest []solrCore
	for _, c := range all {
		if c.Name == s.VectorCollection {
			cols = append(cols, knowledgeProjectCollection{Name: c.Name, DocCount: c.DocCount})
		} else if strings.HasPrefix(c.Name, s.Prefix) {
			rest = append(rest, c)
		}
	}
	for _, c := range rest { // statusCores is name-sorted already
		cols = append(cols, knowledgeProjectCollection{Name: c.Name, DocCount: c.DocCount})
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i := range cols {
		col := &cols[i]
		col.Label, col.Projects = s.label(col.Name), []knowledgeProject{}
		if col.DocCount == 0 {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			if ps, err := s.facetProjects(cctx, col.Name); err != nil {
				col.Error = "facet_failed"
			} else {
				col.Projects = ps
			}
		}()
	}
	wg.Wait()

	title := names()
	distinct := map[string]bool{}
	for i := range cols {
		col := &cols[i]
		for j := range col.Projects {
			col.Projects[j].Name = title[col.Projects[j].Code]
			distinct[col.Projects[j].Code] = true
		}
		sort.Slice(col.Projects, func(a, b int) bool {
			pa, pb := col.Projects[a], col.Projects[b]
			return pa.Docs > pb.Docs || pa.Docs == pb.Docs && pa.Code < pb.Code
		})
		col.ProjectCount = len(col.Projects)
		res.Totals.Docs += col.DocCount
		if col.DocCount == 0 {
			res.Totals.EmptyCollections++
		}
	}
	res.Totals.Collections, res.Totals.Projects, res.Collections = len(cols), len(distinct), cols
	return res, nil
}

var qaseNames struct {
	sync.Mutex
	key  string
	at   time.Time
	data map[string]string
}

// projectNames returns code -> title: Qase titles (cached 10 min), local projects table as fallback. Never fails.
func (a API) projectNames(ctx context.Context) map[string]string {
	key := a.Connector.QaseBaseURL + "|" + a.Connector.QaseToken
	qaseNames.Lock()
	if qaseNames.key == key && qaseNames.data != nil && time.Since(qaseNames.at) < 10*time.Minute {
		defer qaseNames.Unlock()
		return qaseNames.data
	}
	qaseNames.Unlock()
	if m, err := a.Connector.QaseProjectTitles(ctx); err == nil {
		qaseNames.Lock()
		qaseNames.key, qaseNames.at, qaseNames.data = key, time.Now(), m
		qaseNames.Unlock()
		return m
	}
	m := map[string]string{}
	if a.Repo != nil {
		if ps, err := a.Repo.Projects(100000, true, ""); err == nil {
			for _, p := range ps {
				if code := strings.TrimSpace(p.QaseProjectCode); code != "" {
					m[code] = p.Name
				}
			}
		}
	}
	return m
}

func (a API) knowledgeProjects(c echo.Context) error {
	ctx := c.Request().Context()
	out, err := a.Solr.ProjectsByCollection(ctx, func() map[string]string { return a.projectNames(ctx) })
	if err != nil {
		return solrUnavailable(c)
	}
	return c.JSON(http.StatusOK, out)
}
