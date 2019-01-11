[![GoDoc](http://godoc.org/github.com/borderstech/restimator?status.png)](http://godoc.org/github.com/borderstech/restimator)
[![Go Report Card](https://goreportcard.com/badge/github.com/borderstech/restimator)](https://goreportcard.com/report/github.com/borderstech/restimator)

# restimator

Mux-based REST API server container with Postgres support
- Uses GORM for DB interaction with Postgres
- Uses Gorilla Mux for routing
- Uses logmatic for nice server logs

Documentation here: https://godoc.org/github.com/borderstech/restimator

## Example Usage
```go
s := restimator.Server{
    Port: 8080,
    Controllers: [ ... IHttpController ],
    EnableDB: true
}

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

func (c MyController) getStuff(w http.ResponseWriter, r *http.Request) {
    m := Response{Message: "Hello, world!"}
    restimator.SendJSON(w, res)
}

func (c MyController) Load(router *restimator.Router) {
    r := router.NewAPIRouter("/stuff")
    r.Get("/", c.getStuff)
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
