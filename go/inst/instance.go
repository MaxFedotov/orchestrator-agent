package inst

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/github/orchestrator-agent/go/config"
	"github.com/openark/golib/log"
	"github.com/openark/golib/sqlutils"
)

const (
	connectionThreshold = 1
)

// MySQLInfo provides information nesessary for pre-seed checks
type MySQLInfo struct {
	MySQLVersion         string
	IsSlave              bool
	IsMaster             bool
	HasActiveConnections bool
}

func OpenConnection() (*sql.DB, error) {
	mysqlURI := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?interpolateParams=true",
		config.Config.MySQLTopologyUser,
		config.Config.MySQLTopologyPassword,
		"localhost",
		config.Config.MySQLPort,
		"mysql",
	)
	db, _, err := sqlutils.GetDB(mysqlURI)
	if err != nil {
		return nil, err
	}
	err = db.Ping()
	if err != nil {
		return nil, err
	}
	return db, err
}

func QueryData(query string, argsArray []interface{}, on_row func(sqlutils.RowMap) error) error {
	db, err := OpenConnection()
	if err != nil {
		return err
	}
	return log.Criticale(sqlutils.QueryRowsMap(db, query, on_row, argsArray...))
}

func getMySQLVersion() (version string, err error) {
	query := `SELECT @@version;`
	err = QueryData(query, sqlutils.Args(), func(m sqlutils.RowMap) error {
		version = m.GetString("@@version")
		return nil
	})
	if err == nil {
		version = version[0:strings.LastIndex(version, ".")]
	}
	return version, err
}

func isSlave() (isSlave bool, err error) {
	query := "SHOW SLAVE STATUS;"
	err = QueryData(query, sqlutils.Args(), func(m sqlutils.RowMap) error {
		ioThreadRunning := (m.GetString("Slave_IO_Running") == "Yes")
		sqlThreadRunning := (m.GetString("Slave_SQL_Running") == "Yes")
		isSlave = ioThreadRunning && sqlThreadRunning
		return nil
	})
	return isSlave, err
}

func isMaster() (isMaster bool, err error) {
	query := "SHOW SLAVE HOSTS;"
	err = QueryData(query, sqlutils.Args(), func(m sqlutils.RowMap) error {
		isMaster = (len(m.GetString("Slave_UUID")) != 0)
		return nil

	})
	return isMaster, err
}

func hasActiveConnections() (hasActiveConnections bool, err error) {
	query := "SELECT COUNT(*) AS con FROM INFORMATION_SCHEMA.PROCESSLIST WHERE User NOT IN (?,?,?);"
	err = QueryData(query, sqlutils.Args("event_scheduler", "system user", config.Config.MySQLTopologyUser), func(m sqlutils.RowMap) error {
		conCnt := m.GetInt("con")
		if conCnt > connectionThreshold {
			hasActiveConnections = true
		}
		return nil
	})
	return hasActiveConnections, err
}

func GetMySQLInfo() (mysqlinfo MySQLInfo, err error) {
	mysqlinfo.MySQLVersion, err = getMySQLVersion()
	mysqlinfo.IsSlave, err = isSlave()
	mysqlinfo.IsMaster, err = isMaster()
	mysqlinfo.HasActiveConnections, err = hasActiveConnections()
	return mysqlinfo, err
}
