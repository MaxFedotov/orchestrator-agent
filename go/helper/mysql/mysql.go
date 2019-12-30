package mysql

import (
	"database/sql"
	"fmt"

	"github.com/openark/golib/sqlutils"
)

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
	return sqlutils.QueryRowsMap(db, query, onRow, argsArray...)
}
