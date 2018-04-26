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
	IsSlave              bool
	IsMaster             bool
	IsBinlogEnabled      bool
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

func ExecuteQuery(query string, args ...interface{}) (sql.Result, error) {
	var err error
	db, err := OpenConnection()
	if err != nil {
		return nil, err
	}
	res, err := sqlutils.ExecNoPrepare(db, query, args...)
	return res, err
}

func GetMySQLVersion() (Version string, err error) {
	query := `SELECT @@version;`
	err = QueryData(query, sqlutils.Args(), func(m sqlutils.RowMap) error {
		Version = m.GetString("@@version")
		return nil
	})
	if err == nil {
		Version = Version[0:strings.LastIndex(Version, ".")]
	}
	if err != nil {
		log.Errore(err)
	}
	return Version, err
}

func GetMySQLSql_mode() (sqlMode string, err error) {
	query := `SELECT @@sql_mode;`
	err = QueryData(query, sqlutils.Args(), func(m sqlutils.RowMap) error {
		sqlMode = m.GetString("@@sql_mode")
		return nil
	})
	if err != nil {
		log.Errore(err)
	}
	return sqlMode, err
}

func SetMySQLSql_mode(sqlMode string) error {
	query := fmt.Sprintf("SET GLOBAL SQL_MODE='%s';", sqlMode)
	_, err := ExecuteQuery(query)
	return err
}

func isBinlogEnabled() (IsBinlogEnabled bool, err error) {
	query := `SHOW VARIABLES LIKE 'log_bin';`
	err = QueryData(query, sqlutils.Args(), func(m sqlutils.RowMap) error {
		IsBinlogEnabled = (m.GetString("Value") == "ON")
		return nil
	})
	if err != nil {
		log.Errore(err)
	}
	return IsBinlogEnabled, err
}

func isSlave() (IsSlave bool, err error) {
	query := `SHOW SLAVE STATUS;`
	err = QueryData(query, sqlutils.Args(), func(m sqlutils.RowMap) error {
		ioThreadRunning := (m.GetString("Slave_IO_Running") == "Yes")
		sqlThreadRunning := (m.GetString("Slave_SQL_Running") == "Yes")
		IsSlave = ioThreadRunning && sqlThreadRunning
		return nil
	})
	if err != nil {
		log.Errore(err)
	}
	return IsSlave, err
}

func isMaster() (IsMaster bool, err error) {
	query := `SHOW SLAVE HOSTS;`
	err = QueryData(query, sqlutils.Args(), func(m sqlutils.RowMap) error {
		IsMaster = (len(m.GetString("Slave_UUID")) != 0)
		return nil

	})
	if err != nil {
		log.Errore(err)
	}
	return IsMaster, err
}

func hasActiveConnections() (HasActiveConnections bool, err error) {
	query := `SELECT COUNT(*) AS con FROM INFORMATION_SCHEMA.PROCESSLIST WHERE User NOT IN (?,?,?);`
	err = QueryData(query, sqlutils.Args("event_scheduler", "system user", config.Config.MySQLTopologyUser), func(m sqlutils.RowMap) error {
		conCnt := m.GetInt("con")
		if conCnt > connectionThreshold {
			HasActiveConnections = true
		}
		return nil
	})
	if err != nil {
		log.Errore(err)
	}
	return HasActiveConnections, err
}

func getMySQLDatadirPath() (MySQLDatadirPath string, err error) {
	query := `SELECT @@datadir`
	err = QueryData(query, sqlutils.Args(), func(m sqlutils.RowMap) error {
		MySQLDatadirPath = m.GetString("@@datadir")
		return nil
	})
	if err != nil {
		log.Errore(err)
	}
	return MySQLDatadirPath, err
}

func GetMySQLInfo() (Info MySQLInfo, err error) {
	Info.MySQLVersion, err = GetMySQLVersion()
	Info.MySQLDatadirPath, err = getMySQLDatadirPath()
	Info.IsSlave, err = isSlave()
	Info.IsMaster, err = isMaster()
	Info.IsBinlogEnabled, err = isBinlogEnabled()
	Info.HasActiveConnections, err = hasActiveConnections()
	return Info, err
}

func GetMySQLDatabases() (Databases []string, err error) {
	query := `SELECT SCHEMA_NAME FROM information_schema.schemata WHERE SCHEMA_NAME NOT IN ('information_schema','mysql','performance_schema','sys');`
	err = QueryData(query, sqlutils.Args(), func(m sqlutils.RowMap) error {
		db := m.GetString("SCHEMA_NAME")
		Databases = append(Databases, db)
		return nil
	})
	if err != nil {
		log.Errore(err)
	}
	return Databases, err
}

func GetMySQLEngines(dbname string) (Engines []string, err error) {
	query := `SELECT engine FROM information_schema.tables where TABLE_SCHEMA = ? and table_type = 'BASE TABLE' GROUP BY engine;`
	err = QueryData(query, sqlutils.Args(dbname), func(m sqlutils.RowMap) error {
		engine := m.GetString("engine")
		Engines = append(Engines, engine)
		return nil
	})
	if err != nil {
		log.Errore(err)
	}
	return Engines, err
}

func GetTokuDBSize(dbname string) (TokuSize int64, err error) {
	query := `SELECT SUM(bt_size_allocated) AS tables_size FROM information_schema.TokuDB_fractal_tree_info WHERE table_schema = ?;`
	err = QueryData(query, sqlutils.Args(dbname), func(m sqlutils.RowMap) error {
		TokuSize = m.GetInt64("tables_size")
		return nil
	})
	if err != nil {
		log.Errore(err)
	}
	return TokuSize, err
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

func UserExists(username string) (UserExists bool) {
	query := `SELECT user FROM mysql.user WHERE user=?;`
	err := QueryData(query, sqlutils.Args(username), func(m sqlutils.RowMap) error {
		UserExists = (len(m.GetString("user")) != 0)
		return nil
	})
	if err != nil {
		log.Errore(err)
	}
	return UserExists
}

func HasGrant(username string, grant string) (HasGrant bool) {
	query := `SELECT * FROM mysql.user WHERE user=?;`
	err := QueryData(query, sqlutils.Args(username), func(m sqlutils.RowMap) error {
		HasGrant = (m.GetString(grant) == "Y")
		return nil
	})
	if err != nil {
		log.Errore(err)
	}
	return HasGrant
}

func CreateUser(username string, host string, password string) error {
	_, err := ExecuteQuery(`CREATE USER ?@? IDENTIFIED BY ?;`, username, host, password)
	return err
}

func GrantUser(username string, host string, grant string, object string) error {
	query := fmt.Sprintf("GRANT %s ON %s TO '%s'@'%s';", grant, object, username, host)
	_, err := ExecuteQuery(query)
	return err
}

func GenerateBackupForUsers(users []string) (backup string, err error) {
	version, err := GetMySQLVersion()
	if err != nil {
		return "", log.Errore(err)
	}
	if db, err := OpenConnection(); err == nil {
		if version == "5.7" {
			for _, user := range users {
				query := fmt.Sprintf("SHOW CREATE USER %s", user)
				res, err := sqlutils.QueryResultData(db, query)
				if err != nil {
					log.Errore(err)
				}
				backup += res[0][0].String + ";\n"
			}

		}
		for _, user := range users {
			query := fmt.Sprintf("SHOW GRANTS FOR %s", user)
			res, err := sqlutils.QueryResultData(db, query)
			if err != nil {
				log.Errore(err)
			}
			for r := range res {
				backup += res[r][0].String + ";\n"
			}
		}
	}
	return backup, err
}

func ManageReplicationUser() error {
	var err error
	if !UserExists(config.Config.MySQLReplicationUser) {
		err = CreateUser(config.Config.MySQLReplicationUser, "%", config.Config.MySQLReplicationPassword)
		if err != nil {
			return err
		}
	}
	if !HasGrant(config.Config.MySQLReplicationUser, "Repl_slave_priv") {
		err = GrantUser(config.Config.MySQLReplicationUser, "%", "REPLICATION SLAVE", "*.*")
	}
	return err
}

func StartSlave(sourceHost string, sourcePort int, logFile string, position string, gtidPurged string) error {
	var err error
	if len(logFile) > 0 && len(position) > 0 {
		query := fmt.Sprintf(`CHANGE MASTER TO MASTER_LOG_FILE='%s', MASTER_LOG_POS=%s;`, logFile, position)
		_, err = ExecuteQuery(query)
		if err != nil {
			return log.Errore(err)
		}
	}
	if len(gtidPurged) > 0 {
		query := fmt.Sprintf(`SET @@GLOBAL.GTID_PURGED='%s';`, gtidPurged)
		_, err = ExecuteQuery(query)
		if err != nil {
			return log.Errore(err)
		}
	}
	query := fmt.Sprintf(`CHANGE MASTER TO MASTER_HOST='%s', MASTER_USER='%s', MASTER_PASSWORD='%s', MASTER_PORT=%d, MASTER_CONNECT_RETRY=10;`,
		sourceHost, config.Config.MySQLReplicationUser, config.Config.MySQLReplicationPassword, sourcePort)
	_, err = ExecuteQuery(query)
	if err != nil {
		return log.Errore(err)
	}
	query = `START SLAVE;`
	_, err = ExecuteQuery(query)
	if err != nil {
		return log.Errore(err)
	}
	return err
}
