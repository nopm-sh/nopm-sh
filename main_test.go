package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nopm-sh/nopm-sh/core"
	"github.com/stretchr/testify/assert"
)

func TestDownloadURLHashicorpReleaseUrl(t *testing.T) {
	redisClient := core.NewTestRedisClient()
	recipesEngine = core.NewRecipeEngine(redisClient, "")
	router := setupRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/_/providers/hashicorp/release_url/terraform/0.12.18/darwin/amd64", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "https://releases.hashicorp.com/terraform/0.12.18/terraform_0.12.18_darwin_amd64.zip", w.Body.String())
}
