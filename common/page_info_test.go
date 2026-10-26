package common

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func newPageQueryContext(t *testing.T, query string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/?"+query, nil)
	return c
}

func TestGetPageQueryCapsAtDefaultLimit(t *testing.T) {
	c := newPageQueryContext(t, "page_size=200")

	assert.Equal(t, MaxPageSize, GetPageQuery(c).GetPageSize())
}

func TestGetPageQueryWithMaxSizeKeepsChannelsLimit(t *testing.T) {
	c := newPageQueryContext(t, "page_size=200")

	assert.Equal(t, 200, GetPageQueryWithMaxSize(c, 200).GetPageSize())
}

func TestGetPageQueryWithMaxSizeCapsAboveLimit(t *testing.T) {
	c := newPageQueryContext(t, "page_size=500")

	assert.Equal(t, 200, GetPageQueryWithMaxSize(c, 200).GetPageSize())
}

func TestGetPageQueryWithMaxSizeFallsBackToDefaultLimit(t *testing.T) {
	c := newPageQueryContext(t, "page_size=200")

	assert.Equal(t, MaxPageSize, GetPageQueryWithMaxSize(c, 0).GetPageSize())
}
