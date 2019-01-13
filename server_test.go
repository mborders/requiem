package requiem

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type TestController struct {
}

type TestRequest struct {
	Message string
}

func (c TestController) Load(router *Router) {
	r := router.NewAPIRouter("/test")
	r.Get("/one", func(ctx HTTPContext) {
		ctx.SendStatus(http.StatusNoContent)
	})

	r.Post("/two", func(ctx HTTPContext) {
		req := ctx.Body.(*TestRequest)
		ctx.SendJSON(req)
	}, TestRequest{})
}

func TestNewServer(t *testing.T) {
	s := NewServer(TestController{})
	go s.Start()

	timer := time.NewTimer(time.Millisecond * 100)
	<-timer.C

	// Verify endpoint one
	res, _ := http.Get("http://localhost:8080/api/test/one")
	assert.Equal(t, http.StatusNoContent, res.StatusCode, "GET should return 204 status")

	// Verify endpoint two
	m := "Hello"
	b, _ := json.Marshal(TestRequest{Message: m})
	res, _ = http.Post("http://localhost:8080/api/test/two", "application/json", bytes.NewReader(b))

	var result TestRequest
	json.NewDecoder(res.Body).Decode(&result)

	assert.Equal(t, m, result.Message, "POST should have expected response body")
}
