package dbagent

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
		return nil, log.Errore(err)
	}
	err = db.Ping()
	if err != nil {
		return nil, log.Errore(err)
	}
	return db, err
}

func QueryData(query string, argsArray []interface{}, on_row func(sqlutils.RowMap) error) error {
	db, err := OpenConnection()
	if err != nil {
		return log.Errore(err)
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
	if err != nil {
		log.Errore(err)
	}
	return version, err
}

func isSlave() (isSlave bool, err error) {
	query := `SHOW SLAVE STATUS;`
	err = QueryData(query, sqlutils.Args(), func(m sqlutils.RowMap) error {
		ioThreadRunning := (m.GetString("Slave_IO_Running") == "Yes")
		sqlThreadRunning := (m.GetString("Slave_SQL_Running") == "Yes")
		isSlave = ioThreadRunning && sqlThreadRunning
		return nil
	})
	if err != nil {
		log.Errore(err)
	}
	return isSlave, err
}

func isMaster() (isMaster bool, err error) {
	query := `SHOW SLAVE HOSTS;`
	err = QueryData(query, sqlutils.Args(), func(m sqlutils.RowMap) error {
		isMaster = (len(m.GetString("Slave_UUID")) != 0)
		return nil

	})
	if err != nil {
		log.Errore(err)
	}
	return isMaster, err
}

func hasActiveConnections() (hasActiveConnections bool, err error) {
	query := `SELECT COUNT(*) AS con FROM INFORMATION_SCHEMA.PROCESSLIST WHERE User NOT IN (?,?,?);`
	err = QueryData(query, sqlutils.Args("event_scheduler", "system user", config.Config.MySQLTopologyUser), func(m sqlutils.RowMap) error {
		conCnt := m.GetInt("con")
		if conCnt > connectionThreshold {
			hasActiveConnections = true
		}
		return nil
	})
	if err != nil {
		log.Errore(err)
	}
	return hasActiveConnections, err
}

func GetMySQLInfo() (mysqlinfo MySQLInfo, err error) {
	mysqlinfo.MySQLVersion, err = getMySQLVersion()
	mysqlinfo.IsSlave, err = isSlave()
	mysqlinfo.IsMaster, err = isMaster()
	mysqlinfo.HasActiveConnections, err = hasActiveConnections()
	return mysqlinfo, err
}

func GetMySQLDatabases() (databases []string, err error) {
	query := `SELECT SCHEMA_NAME FROM information_schema.schemata WHERE SCHEMA_NAME NOT IN ('information_schema','mysql','performance_schema','sys');`
	err = QueryData(query, sqlutils.Args(), func(m sqlutils.RowMap) error {
		db := m.GetString("SCHEMA_NAME")
		databases = append(databases, db)
		return nil
	})
	if err != nil {
		log.Errore(err)
	}
	return databases, err
}

func GetMySQLEngines(dbname string) (engines []string, err error) {
	query := `SELECT engine FROM information_schema.tables where TABLE_SCHEMA = ? and table_type = 'BASE TABLE' GROUP BY engine;`
	err = QueryData(query, sqlutils.Args(dbname), func(m sqlutils.RowMap) error {
		engine := m.GetString("engine")
		engines = append(engines, engine)
		return nil
	})
	if err != nil {
		log.Errore(err)
	}
	return engines, err
}

func GetTokuDBSize(dbname string) (tokuSize int64, err error) {
	query := `SELECT SUM(bt_size_allocated) AS tables_size FROM information_schema.TokuDB_fractal_tree_info WHERE table_schema = ?;`
	err = QueryData(query, sqlutils.Args(dbname), func(m sqlutils.RowMap) error {
		tokuSize = m.GetInt64("tables_size")
		return nil
	})
	if err != nil {
		log.Errore(err)
	}
	return tokuSize, err
}

func GetInnoDBLogSize() (InnoDBLogSize int64, err error) {
	query := `SELECT @@innodb_log_file_size*@@innodb_log_files_in_group AS logFileSize;`
	err = QueryData(query, sqlutils.Args(), func(m sqlutils.RowMap) error {
		InnoDBLogSize = m.GetInt64("logFileSize")
		return nil
	})
	if err != nil {
		log.Errore(err)
	}
	return InnoDBLogSize, err
}
