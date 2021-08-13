[![GoDoc](http://godoc.org/github.com/mborders/requiem?status.png)](http://godoc.org/github.com/mborders/requiem)
[![Build Status](https://travis-ci.com/mborders/requiem.svg?branch=master)](https://travis-ci.org/mborders/requiem)
[![Go Report Card](https://goreportcard.com/badge/github.com/mborders/requiem)](https://goreportcard.com/report/github.com/mborders/requiem)
[![codecov](https://codecov.io/gh/mborders/requiem/branch/master/graph/badge.svg)](https://codecov.io/gh/mborders/requiem)

# requiem

Controller-based REST API server container for Golang with Postgres support
- Uses [GORM v1.21.13](https://github.com/go-gorm/gorm) for DB interaction with Postgres
- Uses [Gorilla Mux v1.8.0](https://github.com/gorilla/mux) for routing
- Uses [logmatic v0.4.0](https://github.com/mborders/logmatic) for nice server logs
- Default port is 8080
- Default base path is /api

Documentation here: https://godoc.org/github.com/mborders/requiem

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

func AuthInterceptor(ctx HTTPContext) bool {
    // Example:
    //   1) Check if user is authenticated
    //   2) Return false if not
    //   3) Use ctx.SetAttribute() to pass user claim
    
    return true
}

func (c MyController) Load(router *requiem.Router) {
    c.DB = router.DB
    r := router.NewAPIRouter("/stuff")
    r.Get("/", c.getStuff)
    r.Post("/", c.createStuff)
    
    // Use AuthInterceptor
    r.Get("/interceptor", func(ctx HTTPContext) {
        ctx.SendStatus(http.StatusOK)
    }, AuthInterceptor)
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
