[![GoDoc](http://godoc.org/github.com/borderstech/requiem?status.png)](http://godoc.org/github.com/borderstech/requiem)
[![Go Report Card](https://goreportcard.com/badge/github.com/borderstech/requiem)](https://goreportcard.com/report/github.com/borderstech/requiem)

# requiem

Mux-based REST API server container with Postgres support
- Uses GORM for DB interaction with Postgres
- Uses Gorilla Mux for routing
- Uses logmatic for nice server logs

Documentation here: https://godoc.org/github.com/borderstech/requiem

## Example Usage
### Without DB
```go
s := requiem.NewServer(... controllers)
s.Start()
```

### With DB
```go
s := requiem.NewServer(... controllers)
s.EnableDB = true
s.Start()
```

### Change port or base path
```go
s := requiem.NewServer(... controllers)
s.Port = 9090
s.BasePath = "/rest"
s.Start()
```

## HttpController example
```go
type MyController struct {
    DB *gorm.DB
}

type Response struct {
    Message string
}

type CreateRequest struct {
    SomeValue string
}

func (c MyController) getStuff(ctx requiem.HTTPContext) {
    m := Response{Message: "Hello, world!"}
    ctx.SendJSON(res)
}

func (c MyController) createStuff(ctx requiem.HTTPContext) {
    m := ctx.Body.(*CreateRequest)
    fmt.Println("Value: %s", m.SomeValue)
    ctx.SendStatus(Http.StatusNoContent)
}

func (c MyController) Load(router *requiem.Router) {
    c.DB = router.DB
    r := router.NewAPIRouter("/stuff")
    r.Get("/", c.getStuff)
    r.Post("/", c.createStuff)
}
```

## DB Connection Environment Variables (if DB is enabled)
```
DB_HOST
DB_PORT
DB_NAME
DB_USERNAME
DB_PASSWORD
```
