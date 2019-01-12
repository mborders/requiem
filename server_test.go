package requiem

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type TestController struct {
}

type TestResponse struct {
}

func (c TestController) Load(router *Router) {
	r := router.NewAPIRouter("/test")
	r.Get("/one", func(ctx HTTPContext) {
		ctx.SendStatus(http.StatusNoContent)
	})

	r.Post("/two", func(ctx HTTPContext) {
		ctx.SendStatus(http.StatusAccepted)
	}, TestResponse{})
}

func TestNewServer(t *testing.T) {
	s := NewServer(TestController{})
	go s.Start()

	timer := time.NewTimer(time.Second)
	<-timer.C

	res, _ := http.Get("http://localhost:8080/api/test/one")
	assert.Equal(t, http.StatusNoContent, res.StatusCode, "GET should return 204 status")

	res, _ = http.Post("http://localhost:8080/api/test/two", "application/json", nil)
	assert.Equal(t, http.StatusAccepted, res.StatusCode, "POST should return 202 status")
}
