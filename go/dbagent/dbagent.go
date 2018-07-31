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
	MySQLDatadirPath     string
	MySQLBackupdirPath   string
	HasActiveConnections bool
}

func OpenConnection() (*sql.DB, error) {
	config.Config.RLock()
	defer config.Config.RUnlock()
	mysqlURI := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?interpolateParams=true&timeout=1s",
		config.Config.MySQLTopologyUser,
		config.Config.MySQLTopologyPassword,
		"localhost",
		config.Config.MySQLPort,
		"mysql",
	)
	db, _, err := sqlutils.GetDB(mysqlURI)
	db.SetMaxIdleConns(0)
	err = db.Ping()
	return db, err
}

func QueryData(query string, argsArray []interface{}, on_row func(sqlutils.RowMap) error) error {
	db, err := OpenConnection()
	if err != nil {
		return err
	}
	return sqlutils.QueryRowsMap(db, query, on_row, argsArray...)
}

func QueryRowString(query string) (result string, err error) {
	db, err := OpenConnection()
	if err != nil {
		return result, err
	}
	if err := db.QueryRow(query).Scan(&result); err != nil {
		return result, err
	}
	return result, err
}

func ExecuteQuery(query string, args ...interface{}) (sql.Result, error) {
	var err error
	db, err := OpenConnection()
	if err != nil {
		return nil, err
	}
	res, err := sqlutils.ExecNoPrepare(db, query, args...)
	return res, err
}

func GetMySQLVersion() (version string, err error) {
	query := `SELECT @@version;`
	version, err = QueryRowString(query)
	if err == nil {
		version = version[0:strings.LastIndex(version, ".")]
	}
	return version, log.Errore(err)
}

func GetMySQLSql_mode() (sqlMode string, err error) {
	query := `SELECT @@sql_mode;`
	sqlMode, err = QueryRowString(query)
	return sqlMode, log.Errore(err)
}

func SetMySQLSql_mode(sqlMode string) error {
	query := fmt.Sprintf("SET GLOBAL SQL_MODE='%s';", sqlMode)
	_, err := ExecuteQuery(query)
	return err
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
	return hasActiveConnections, log.Errore(err)
}

func GetMySQLInfo() (info MySQLInfo, err error) {
	info.MySQLVersion, err = GetMySQLVersion()
	info.MySQLDatadirPath = config.Config.MySQLDataDir
	info.MySQLBackupdirPath = config.Config.MySQLBackupDir
	info.HasActiveConnections, err = hasActiveConnections()
	return info, err
}

func GetMySQLDatabases() (databases []string, err error) {
	query := `SELECT SCHEMA_NAME FROM information_schema.schemata WHERE SCHEMA_NAME NOT IN ('information_schema','mysql','performance_schema','sys');`
	err = QueryData(query, sqlutils.Args(), func(m sqlutils.RowMap) error {
		db := m.GetString("SCHEMA_NAME")
		databases = append(databases, db)
		return nil
	})
	return databases, log.Errore(err)
}

func GetMySQLEngines(dbname string) (engines []string, err error) {
	query := `SELECT engine FROM information_schema.tables where TABLE_SCHEMA = ? and table_type = 'BASE TABLE' GROUP BY engine;`
	err = QueryData(query, sqlutils.Args(dbname), func(m sqlutils.RowMap) error {
		engine := m.GetString("engine")
		engines = append(engines, engine)
		return nil
	})
	return engines, log.Errore(err)
}

func GetTokuDBSize(dbname string) (tokuSize int64, err error) {
	query := `SELECT SUM(bt_size_allocated) AS tables_size FROM information_schema.TokuDB_fractal_tree_info WHERE table_schema = ?;`
	err = QueryData(query, sqlutils.Args(dbname), func(m sqlutils.RowMap) error {
		tokuSize = m.GetInt64("tables_size")
		return nil
	})
	return tokuSize, log.Errore(err)
}

func GetInnoDBLogSize() (innoDBLogSize int64, err error) {
	query := `SELECT @@innodb_log_file_size*@@innodb_log_files_in_group AS logFileSize;`
	err = QueryData(query, sqlutils.Args(), func(m sqlutils.RowMap) error {
		innoDBLogSize = m.GetInt64("logFileSize")
		return nil
	})
	return innoDBLogSize, log.Errore(err)
}

func GenerateBackupForUsers(users []string) (backup string, err error) {
	if db, err := OpenConnection(); err == nil {
		for _, user := range users {
			// small hack cos MySQL 5.6 doesn't have DROP USER IF EXISTS. USAGE is synonym for "no privileges"
			backup += fmt.Sprintf("GRANT USAGE ON *.* TO %s IDENTIFIED BY 'temporary_password'; DROP USER %s;", user, user)
			query := fmt.Sprintf("SHOW CREATE USER %s", user)
			res, err := sqlutils.QueryResultData(db, query)
			if err != nil {
				log.Errore(err)
			} else {
				backup += res[0][0].String + ";\n"
			}
			query = fmt.Sprintf("SHOW GRANTS FOR %s", user)
			res, err = sqlutils.QueryResultData(db, query)
			if err != nil {
				log.Errore(err)
			} else {
				for r := range res {
					backup += res[r][0].String + ";\n"
				}
			}
		}
	}
	return backup, log.Errore(err)
}

func SetReplicationUserAndPassword() error {
	query := fmt.Sprintf(`CHANGE MASTER TO MASTER_USER='%s', MASTER_PASSWORD='%s';`,
		config.Config.MySQLReplicationUser, config.Config.MySQLReplicationPassword)
	_, err := ExecuteQuery(query)
	return err
}
