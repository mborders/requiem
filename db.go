package requiem

import (
	"fmt"

	"github.com/caarlos0/env"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres" // GORM PGSQL dialect
	_ "github.com/lib/pq"                        // PGSQL driver
)

type dbConfig struct {
	Host         string `env:"DB_HOST"`
	Port         string `env:"DB_PORT"`
	Username     string `env:"DB_USERNAME"`
	Password     string `env:"DB_PASSWORD"`
	DatabaseName string `env:"DB_NAME"`
}

func loadDBConfig() dbConfig {
	cfg := dbConfig{}
	err := env.Parse(&cfg)
	if err != nil {
		if ExitOnFatal {
			Logger.Fatal("Could not load DB config: %s", err.Error())
		} else {
			Logger.Error("FATAL Could not load DB config: %s", err.Error())
		}
	}

	return cfg
}

// newDBConnection obtains DB connection details from environment variables
// and initializes the DB connection.
func newDBConnection() *gorm.DB {
	cfg := loadDBConfig()
	db, err := gorm.Open("postgres", fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.DatabaseName))
	if err != nil {
		if ExitOnFatal {
			Logger.Fatal("Could not connect to DB %s", err)
		} else {
			Logger.Error("FATAL Could not connect to DB %s", err)
		}
	}

	return db
}
