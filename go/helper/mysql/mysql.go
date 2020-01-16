package mysql

import (
	"database/sql"
	"fmt"

	"github.com/openark/golib/sqlutils"
	log "github.com/sirupsen/logrus"
)

var logger = log.WithFields(log.Fields{"prefix": "MYSQL"})

// MySQLClient describes MySQL connection
type MySQLClient struct {
	Conn *sql.DB
}

func OpenConnection(user string, password string, port int) (*sql.DB, error) {
	mysqlURI := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?interpolateParams=true&timeout=1s",
		user,
		password,
		"localhost",
		port,
		"mysql",
	)
	db, _, err := sqlutils.GetDB(mysqlURI)
	db.SetMaxIdleConns(0)
	err = db.Ping()
	return db, err
}

func QueryData(db *sql.DB, query string, argsArray []interface{}, onRow func(sqlutils.RowMap) error) error {
	logger.WithFields(log.Fields{"query": query, "params": argsArray}).Debug("Query executed")
	return sqlutils.QueryRowsMap(db, query, onRow, argsArray...)
}
