package monitoring

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Solr is a small read-only client for the vector DB behind Knowledge & RAG
// monitoring. Errors never carry the URL or credentials (see get).
type Solr struct {
	Client                          *http.Client
	BaseURL, User, Password         string
	VectorCollection, Prefix, Model string
	Dimension                       int
	Targets                         map[string]int64
	StaleAfter                      time.Duration
	AutoSyncEvery, WebhookURL       string
}

type solrCore struct {
	Name         string
	DocCount     int64
	SizeBytes    int64
	LastModified *time.Time
}

func NewSolr() Solr {
	dim, _ := strconv.Atoi(os.Getenv("SOLR_VECTOR_DIMENSION"))
	timeout, err := time.ParseDuration(os.Getenv("SOLR_TIMEOUT"))
	if err != nil || timeout <= 0 {
		timeout = 10 * time.Second
	}
	return Solr{
		Client: &http.Client{Timeout: timeout}, BaseURL: strings.TrimRight(os.Getenv("SOLR_BASE_URL"), "/"),
		User: os.Getenv("SOLR_USER"), Password: os.Getenv("SOLR_PASSWORD"),
		VectorCollection: os.Getenv("SOLR_VECTOR_COLLECTION"), Prefix: defaultString(os.Getenv("SOLR_COLLECTION_PREFIX"), "tc_"),
		Model: os.Getenv("SOLR_EMBEDDING_MODEL"), Dimension: dim,
		Targets: parseTargets(os.Getenv("SOLR_COLLECTION_TARGETS")), StaleAfter: parseStaleAfter(os.Getenv("SOLR_SYNC_STALE_AFTER")),
		AutoSyncEvery: os.Getenv("SOLR_AUTO_SYNC_EVERY"), WebhookURL: os.Getenv("SOLR_SYNC_WEBHOOK_URL"),
	}
}

// parseStaleAfter accepts time.ParseDuration syntax plus a "Nd" days suffix; default 14d.
func parseStaleAfter(s string) time.Duration {
	s = strings.TrimSpace(s)
	if n, ok := strings.CutSuffix(s, "d"); ok {
		if days, err := strconv.ParseFloat(n, 64); err == nil && days > 0 {
			return time.Duration(days * 24 * float64(time.Hour))
		}
	}
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return d
	}
	return 14 * 24 * time.Hour
}

func parseTargets(s string) map[string]int64 {
	out := map[string]int64{}
	for _, pair := range strings.Split(s, ",") {
		name, v, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); ok && err == nil && n > 0 {
			out[strings.TrimSpace(name)] = n
		}
	}
	return out
}

func (s Solr) get(ctx context.Context, path string, params url.Values, out any) error {
	if s.BaseURL == "" {
		return errors.New("solr not configured")
	}
	params.Set("wt", "json")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.BaseURL+path+"?"+params.Encode(), nil)
	if err != nil {
		return errors.New("solr request failed")
	}
	if s.User != "" || s.Password != "" {
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(s.User+":"+s.Password)))
	}
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil { // *url.Error embeds the URL, so do not wrap it
		return errors.New("solr unreachable")
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("solr status %d", res.StatusCode)
	}
	if json.NewDecoder(res.Body).Decode(out) != nil {
		return errors.New("solr bad response")
	}
	return nil
}

// Cores lists prefix-matching cores (dynamic), excluding the vector collection.
func (s Solr) Cores(ctx context.Context) ([]solrCore, error) {
	all, err := s.statusCores(ctx)
	if err != nil {
		return nil, err
	}
	cores := []solrCore{}
	for _, c := range all {
		if strings.HasPrefix(c.Name, s.Prefix) && c.Name != s.VectorCollection {
			cores = append(cores, c)
		}
	}
	return cores, nil
}

// statusCores returns every core from the cores STATUS call, sorted by name.
func (s Solr) statusCores(ctx context.Context) ([]solrCore, error) {
	var raw struct {
		Status map[string]struct {
			Index struct {
				NumDocs      int64  `json:"numDocs"`
				SizeInBytes  int64  `json:"sizeInBytes"`
				LastModified string `json:"lastModified"`
			} `json:"index"`
		} `json:"status"`
	}
	if err := s.get(ctx, "/admin/cores", url.Values{"action": {"STATUS"}}, &raw); err != nil {
		return nil, err
	}
	cores := []solrCore{}
	for name, st := range raw.Status {
		core := solrCore{Name: name, DocCount: st.Index.NumDocs, SizeBytes: st.Index.SizeInBytes}
		if t, err := time.Parse(time.RFC3339, st.Index.LastModified); err == nil {
			core.LastModified = &t
		}
		cores = append(cores, core)
	}
	sort.Slice(cores, func(i, j int) bool { return cores[i].Name < cores[j].Name })
	return cores, nil
}

type knowledgeCollection struct {
	Name         string     `json:"name"`
	DocCount     int64      `json:"docCount"`
	Target       *int64     `json:"target"`
	CoveragePct  *float64   `json:"coveragePct"`
	Status       string     `json:"status"`
	LastSyncedAt *time.Time `json:"lastSyncedAt"`
	OutdatedDocs int64      `json:"outdatedDocs"`
	SizeBytes    int64      `json:"sizeBytes"`
}

func (s Solr) collection(c solrCore, now time.Time) knowledgeCollection {
	out := knowledgeCollection{Name: c.Name, DocCount: c.DocCount, SizeBytes: c.SizeBytes, LastSyncedAt: c.LastModified}
	if t, ok := s.Targets[c.Name]; ok {
		pct := float64(int(float64(c.DocCount)/float64(t)*1000+0.5)) / 10
		out.Target, out.CoveragePct = &t, &pct
	}
	switch {
	case c.DocCount == 0 || out.CoveragePct != nil && *out.CoveragePct < 70:
		out.Status = "NEEDS_REINDEX"
	case c.LastModified == nil || now.Sub(*c.LastModified) > s.StaleAfter:
		out.Status = "OUTDATED_SYNC"
	default: // ponytail: 70-90% coverage is not specified; treated as HEALTHY
		out.Status = "HEALTHY"
	}
	return out
}

func (s Solr) Collections(ctx context.Context) ([]knowledgeCollection, error) {
	cores, err := s.Cores(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]knowledgeCollection, 0, len(cores))
	for _, c := range cores {
		out = append(out, s.collection(c, time.Now()))
	}
	return out, nil
}

type knowledgeOverview struct {
	RAM struct {
		UsedGb  float64 `json:"usedGb"`
		TotalGb float64 `json:"totalGb"`
		Pct     float64 `json:"pct"`
	} `json:"ram"`
	LastFullSync *time.Time `json:"lastFullSync"`
	Embedding    struct {
		Model     string `json:"model"`
		Dimension int    `json:"dimension"`
	} `json:"embedding"`
	CollectionsActive int    `json:"collectionsActive"`
	AutoSyncEvery     string `json:"autoSyncEvery"`
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }

func (s Solr) Overview(ctx context.Context) (knowledgeOverview, error) {
	var out knowledgeOverview
	var sys struct {
		JVM struct {
			Memory struct {
				Raw struct {
					Used float64 `json:"used"`
					Max  float64 `json:"max"`
				} `json:"raw"`
			} `json:"memory"`
		} `json:"jvm"`
	}
	if err := s.get(ctx, "/admin/info/system", url.Values{}, &sys); err != nil {
		return out, err
	}
	const gb = 1 << 30
	raw := sys.JVM.Memory.Raw // heap, shown by the UI as "RAM / Heap"
	out.RAM.UsedGb, out.RAM.TotalGb = round1(raw.Used/gb), round1(raw.Max/gb)
	if raw.Max > 0 {
		out.RAM.Pct = round1(raw.Used / raw.Max * 100)
	}
	cores, err := s.Cores(ctx)
	if err != nil {
		return out, err
	}
	for _, c := range cores {
		if c.DocCount > 0 {
			out.CollectionsActive++
		}
		// lastFullSync = newest lastModified across collections.
		if c.LastModified != nil && (out.LastFullSync == nil || c.LastModified.After(*out.LastFullSync)) {
			out.LastFullSync = c.LastModified
		}
	}
	out.Embedding.Model, out.Embedding.Dimension, out.AutoSyncEvery = s.Model, s.dimension(ctx), s.AutoSyncEvery
	return out, nil
}

// dimension reads DenseVectorField.vectorDimension from the vector collection schema, falling back to env.
func (s Solr) dimension(ctx context.Context) int {
	var schema struct {
		FieldTypes []struct {
			Class     string `json:"class"`
			Dimension int    `json:"vectorDimension"`
		} `json:"fieldTypes"`
	}
	if s.get(ctx, "/"+url.PathEscape(s.VectorCollection)+"/schema/fieldtypes", url.Values{}, &schema) == nil {
		for _, ft := range schema.FieldTypes {
			if ft.Class == "solr.DenseVectorField" && ft.Dimension > 0 {
				return ft.Dimension
			}
		}
	}
	return s.Dimension
}

type knowledgeDocument struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	Key          string     `json:"key"`
	SourceURL    string     `json:"sourceUrl"`
	Collection   string     `json:"collection"`
	Chunks       int        `json:"chunks"`
	Dims         int        `json:"dims"`
	LastSyncedAt *time.Time `json:"lastSyncedAt"`
	SyncStatus   string     `json:"syncStatus"`
}

var solrSpecial = strings.NewReplacer(`\`, `\\`, `+`, `\+`, `-`, `\-`, `&`, `\&`, `|`, `\|`, `!`, `\!`, `(`, `\(`, `)`, `\)`,
	`{`, `\{`, `}`, `\}`, `[`, `\[`, `]`, `\]`, `^`, `\^`, `"`, `\"`, `~`, `\~`, `*`, `\*`, `?`, `\?`, `:`, `\:`, `/`, `\/`, ` `, `\ `)

// firstString returns v when it is a string, or the first element when Solr gives a multi-valued array.
func firstString(v any) string {
	if a, ok := v.([]any); ok && len(a) > 0 {
		v = a[0]
	}
	s, _ := v.(string)
	return s
}

// Documents pages docs of one core. With no collection it queries the vector
// collection (one place holding every doc; merging pages across cores would
// make total/paging wrong). Each doc's collection is then prefix+lower(project)
// when that core exists, else the vector collection name.
func (s Solr) Documents(ctx context.Context, collection, q string, page, pageSize int) (any, bool, error) {
	cores, err := s.Cores(ctx)
	if err != nil {
		return nil, false, err
	}
	known := map[string]solrCore{}
	for _, c := range cores {
		known[c.Name] = c
	}
	core := s.VectorCollection
	if collection != "" {
		if _, ok := known[collection]; !ok {
			return nil, false, nil
		}
		core = collection
	}
	query := "*:*"
	if q = strings.TrimSpace(q); q != "" {
		e := solrSpecial.Replace(q)
		query = "title:*" + e + "* OR id:*" + e + "*"
	}
	var raw struct {
		Response struct {
			NumFound int64            `json:"numFound"`
			Docs     []map[string]any `json:"docs"`
		} `json:"response"`
	}
	params := url.Values{"q": {query}, "fl": {"id,title,source,project"}, "sort": {"id asc"},
		"start": {strconv.Itoa((page - 1) * pageSize)}, "rows": {strconv.Itoa(pageSize)}}
	if err := s.get(ctx, "/"+url.PathEscape(core)+"/select", params, &raw); err != nil {
		return nil, true, err
	}
	dim := s.dimension(ctx)
	items := make([]knowledgeDocument, 0, len(raw.Response.Docs))
	for _, d := range raw.Response.Docs {
		id := firstString(d["id"])
		coll := core
		if collection == "" {
			if c, ok := known[s.Prefix+strings.ToLower(firstString(d["project"]))]; ok {
				coll = c.Name
			}
		}
		item := knowledgeDocument{ID: id, Title: firstString(d["title"]), Key: id, SourceURL: firstString(d["source"]), Collection: coll,
			Chunks: 1, Dims: dim, SyncStatus: "SYNCED"}
		if c, ok := known[coll]; ok { // Solr docs carry no per-doc timestamp; use the collection's
			item.LastSyncedAt = c.LastModified
		}
		items = append(items, item)
	}
	return map[string]any{"items": items, "total": raw.Response.NumFound, "page": page, "pageSize": pageSize}, true, nil
}
