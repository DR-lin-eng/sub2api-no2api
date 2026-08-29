package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBindCustomModelJSONRejectsOversizedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/custom-model-configs/templates",
		strings.NewReader(`{"name":"`+strings.Repeat("x", maxCustomModelConfigBodyBytes)+`"}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")

	var target map[string]any
	require.False(t, bindCustomModelJSON(c, &target))
	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
}

func TestParseOptionalCustomModelFields(t *testing.T) {
	id, set, err := parseOptionalCustomModelID([]byte("42"))
	require.NoError(t, err)
	require.True(t, set)
	require.Equal(t, int64(42), *id)

	id, set, err = parseOptionalCustomModelID([]byte("null"))
	require.NoError(t, err)
	require.True(t, set)
	require.Nil(t, id)

	value, set, err := parseOptionalString([]byte("null"))
	require.NoError(t, err)
	require.True(t, set)
	require.Equal(t, "", *value)

	_, _, err = parseOptionalCustomModelID([]byte("0"))
	require.Error(t, err)
}
