package monitoring

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

func (a API) RegisterKnowledgeRoutes(group *echo.Group) {
	group.GET("/knowledge/overview", a.knowledgeOverview)
	group.GET("/knowledge/collections", a.knowledgeCollections)
	group.GET("/knowledge/documents", a.knowledgeDocuments)
	group.POST("/knowledge/collections/:name/sync", a.knowledgeTrigger("sync"), a.managerAuth)
	group.POST("/knowledge/collections/:name/reindex", a.knowledgeTrigger("reindex"), a.managerAuth)
}

func solrUnavailable(c echo.Context) error {
	return c.JSON(http.StatusBadGateway, map[string]string{"code": "SOLR_UNAVAILABLE", "message": "knowledge store is unavailable"})
}

func collectionNotFound(c echo.Context) error {
	return c.JSON(http.StatusNotFound, map[string]string{"code": "COLLECTION_NOT_FOUND", "message": "collection does not exist"})
}

func (a API) knowledgeOverview(c echo.Context) error {
	out, err := a.Solr.Overview(c.Request().Context())
	if err != nil {
		return solrUnavailable(c)
	}
	return c.JSON(http.StatusOK, out)
}

func (a API) knowledgeCollections(c echo.Context) error {
	out, err := a.Solr.Collections(c.Request().Context())
	if err != nil {
		return solrUnavailable(c)
	}
	return c.JSON(http.StatusOK, out)
}

func (a API) knowledgeDocuments(c echo.Context) error {
	page, pageSize := documentPage(c)
	out, found, err := a.Solr.Documents(c.Request().Context(), strings.TrimSpace(c.QueryParam("collection")), c.QueryParam("q"), page, pageSize)
	if !found && err == nil {
		return collectionNotFound(c)
	}
	if err != nil {
		return solrUnavailable(c)
	}
	return c.JSON(http.StatusOK, out)
}

// knowledgeTrigger forwards a sync/reindex request to the external indexing
// pipeline; this repo does not run embedding itself.
func (a API) knowledgeTrigger(mode string) echo.HandlerFunc {
	return func(c echo.Context) error {
		if a.Solr.WebhookURL == "" {
			return c.JSON(http.StatusNotImplemented, map[string]string{"code": "SYNC_NOT_CONFIGURED", "message": "sync pipeline webhook is not configured"})
		}
		name := c.Param("name")
		cores, err := a.Solr.Cores(c.Request().Context())
		if err != nil {
			return solrUnavailable(c)
		}
		found := false
		for _, core := range cores {
			found = found || core.Name == name
		}
		if !found {
			return collectionNotFound(c)
		}
		body, _ := json.Marshal(map[string]string{"collection": name, "mode": mode})
		req, err := http.NewRequestWithContext(c.Request().Context(), http.MethodPost, a.Solr.WebhookURL, bytes.NewReader(body))
		if err != nil {
			return c.JSON(http.StatusBadGateway, map[string]string{"code": "SYNC_TRIGGER_FAILED", "message": "sync pipeline request failed"})
		}
		req.Header.Set("Content-Type", "application/json")
		client := a.Solr.Client
		if client == nil {
			client = http.DefaultClient
		}
		res, err := client.Do(req)
		if err == nil {
			res.Body.Close()
		}
		if err != nil || res.StatusCode/100 != 2 {
			return c.JSON(http.StatusBadGateway, map[string]string{"code": "SYNC_TRIGGER_FAILED", "message": "sync pipeline request failed"})
		}
		return c.JSON(http.StatusAccepted, map[string]string{"collection": name, "mode": mode, "status": "ACCEPTED"})
	}
}
