package mysql

import (
	"bytes"
	"database/sql"
	"fmt"

	"github.com/openark/golib/sqlutils"
	log "github.com/sirupsen/logrus"
)

type Engine int

const (
	ROCKSDB Engine = iota
	MRG_MYISAM
	CSV
	BLACKHOLE
	InnoDB
	MEMORY
	ARCHIVE
	MyISAM
	FEDERATED
	TokuDB
)

func (e Engine) String() string {
	return [...]string{"ROCKSDB", "MRG_MYISAM", "CSV", "BLACKHOLE", "InnoDB", "MEMORY", "ARCHIVE", "MyISAM", "FEDERATED", "TokuDB"}[e]
}

// MarshalJSON marshals the enum as a quoted json string
func (e Engine) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString(`"`)
	buffer.WriteString(e.String())
	buffer.WriteString(`"`)
	return buffer.Bytes(), nil
}

var ToEngine = map[string]Engine{
	"ROCKSDB":    ROCKSDB,
	"MRG_MYISAM": MRG_MYISAM,
	"CSV":        CSV,
	"BLACKHOLE":  BLACKHOLE,
	"InnoDB":     InnoDB,
	"MEMORY":     MEMORY,
	"ARCHIVE":    ARCHIVE,
	"MyISAM":     MyISAM,
	"FEDERATED":  FEDERATED,
	"TokuDB":     TokuDB,
}

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

func Exec(db *sql.DB, query string) error {
	logger.WithFields(log.Fields{"query": query}).Debug("Query executed")
	_, err := sqlutils.ExecNoPrepare(db, query)
	return err
}
