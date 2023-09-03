package requiem

type HealthcheckController struct {
}

func (c HealthcheckController) healthcheck(ctx HTTPContext) {
	ctx.SendStatus(200)
}

func (c HealthcheckController) Load(router *Router) {
	r := router.NewRestRouter("/healthcheck")
	r.Get("", c.healthcheck)
}
