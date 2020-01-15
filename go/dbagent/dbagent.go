/*
   Copyright 2014 Outbrain Inc.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package dbagent

import (
	"fmt"
	"regexp"

	"github.com/github/orchestrator-agent/go/helper/mysql"
	"github.com/openark/golib/sqlutils"
)

// MySQLDatabase describes a MySQL database
type MySQLDatabase struct {
	Engines []string
	Size    int64
}

// NewMySQLClient initialize new MySQL Client
func NewMySQLClient(user string, password string, port int) (*mysql.MySQLClient, error) {
	mysqlClient := &mysql.MySQLClient{}
	conn, err := mysql.OpenConnection(user, password, port)
	if err != nil {
		return mysqlClient, err
	}
	mysqlClient.Conn = conn
	return mysqlClient, nil
}

func getDatabases(m *mysql.MySQLClient) (databases []string, err error) {
	query := `SELECT SCHEMA_NAME FROM information_schema.schemata WHERE SCHEMA_NAME NOT IN ('information_schema','mysql','performance_schema','sys');`
	err = mysql.QueryData(m.Conn, query, sqlutils.Args(), func(m sqlutils.RowMap) error {
		db := m.GetString("SCHEMA_NAME")
		databases = append(databases, db)
		return nil
	})
	return databases, err
}

func getEngines(m *mysql.MySQLClient, dbname string) (engines []string, err error) {
	query := `SELECT engine FROM information_schema.tables where TABLE_SCHEMA = ? and table_type = 'BASE TABLE' GROUP BY engine;`
	err = mysql.QueryData(m.Conn, query, sqlutils.Args(dbname), func(m sqlutils.RowMap) error {
		engine := m.GetString("engine")
		engines = append(engines, engine)
		return nil
	})
	return engines, err
}

func getDatabaseSize(m *mysql.MySQLClient, dbname string) (size int64, err error) {
	query := `SELECT SUM(data_length+index_length+data_free) AS "size" FROM information_schema.tables where TABLE_SCHEMA = ?;`
	err = mysql.QueryData(m.Conn, query, sqlutils.Args(dbname), func(m sqlutils.RowMap) error {
		size = m.GetInt64("size")
		return nil
	})
	return size, err
}

// GetMySQLDatabases return information about MySQL databases, size and engines
func GetMySQLDatabases(m *mysql.MySQLClient) (dbinfo map[string]*MySQLDatabase, err error) {
	dbinfo = make(map[string]*MySQLDatabase)
	databases, err := getDatabases(m)
	if err != nil {
		return dbinfo, fmt.Errorf("Unable to get databases info: %+v", err)
	}
	for _, db := range databases {
		engines, err := getEngines(m, db)
		if err != nil {
			return dbinfo, fmt.Errorf("Unable to get enigines info: %+v", err)
		}
		size, err := getDatabaseSize(m, db)
		if err != nil {
			return dbinfo, fmt.Errorf("Unable to get databases size info: %+v", err)
		}
		dbinfo[db] = &MySQLDatabase{engines, size}
	}
	return dbinfo, err
}

// GetMySQLDatadir returns path to MySQL data directory
func GetMySQLDatadir(m *mysql.MySQLClient) (datadir string, err error) {
	query := `SHOW VARIABLES LIKE 'datadir'`
	err = mysql.QueryData(m.Conn, query, sqlutils.Args(), func(m sqlutils.RowMap) error {
		datadir = m.GetString("Value")
		return nil
	})
	return datadir, err
}

// GetMySQLLogFile returns path to MySQL log file
func GetMySQLLogFile(m *mysql.MySQLClient) (logFile string, err error) {
	query := `SHOW VARIABLES LIKE 'log_error'`
	err = mysql.QueryData(m.Conn, query, sqlutils.Args(), func(m sqlutils.RowMap) error {
		logFile = m.GetString("Value")
		return nil
	})
	return logFile, err
}

// GetMySQLVersion return version of installed MySQL
func GetMySQLVersion(m *mysql.MySQLClient) (version string, err error) {
	query := `SELECT @@version AS version`
	err = mysql.QueryData(m.Conn, query, sqlutils.Args(), func(m sqlutils.RowMap) error {
		version = m.GetString("version")
		return nil
	})
	re := regexp.MustCompile(`(\d+)\.(\d+)`)
	majorVersion := re.FindStringSubmatch(version)[0]
	return majorVersion, err
}
