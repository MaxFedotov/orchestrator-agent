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

package agent

import (
	"fmt"

	"github.com/github/orchestrator-agent/go/config"
	"github.com/github/orchestrator-agent/go/helper/functions"
	"github.com/openark/golib/sqlutils"
)

func getInnoDBLogSize(user string, password string, port int) (innoDBLogSize int64, err error) {
	query := `SELECT @@innodb_log_file_size*@@innodb_log_files_in_group AS logFileSize;`
	err = functions.QueryData(user, password, port, query, sqlutils.Args(), func(m sqlutils.RowMap) error {
		innoDBLogSize = m.GetInt64("logFileSize")
		return nil
	})
	return innoDBLogSize, err
}

func getDatabases(user string, password string, port int) (databases []string, err error) {
	query := `SELECT SCHEMA_NAME FROM information_schema.schemata WHERE SCHEMA_NAME NOT IN ('information_schema','mysql','performance_schema','sys');`
	err = functions.QueryData(user, password, port, query, sqlutils.Args(), func(m sqlutils.RowMap) error {
		db := m.GetString("SCHEMA_NAME")
		databases = append(databases, db)
		return nil
	})
	return databases, err
}

func getEngines(user string, password string, port int, dbname string) (engines []string, err error) {
	query := `SELECT engine FROM information_schema.tables where TABLE_SCHEMA = ? and table_type = 'BASE TABLE' GROUP BY engine;`
	err = functions.QueryData(user, password, port, query, sqlutils.Args(dbname), func(m sqlutils.RowMap) error {
		engine := m.GetString("engine")
		engines = append(engines, engine)
		return nil
	})
	return engines, err
}

func getDatabaseSize(user string, password string, port int, dbname string) (size int64, dataSize int64, err error) {
	query := `SELECT SUM(data_length+index_length+data_free) AS "size", SUM(data_length) AS "dataSize" FROM information_schema.tables where TABLE_SCHEMA = ?;`
	err = functions.QueryData(user, password, port, query, sqlutils.Args(dbname), func(m sqlutils.RowMap) error {
		size = m.GetInt64("size")
		dataSize = m.GetInt64("dataSize")
		return nil
	})
	return size, dataSize, err
}

// These magical multiplies for logicalSize (0.6 in case of compression, 0.8 in other cases) are just raw estimates. They can be wrong, but we will use them
// as 'we should have at least' space check, because we can't make any accurate estimations for logical backups
func getMySQLDatabases(user string, password string, port int) (dbinfo map[string]*MySQLDatabase, err error) {
	var logicalSize int64
	dbinfo = make(map[string]*MySQLDatabase)
	databases, err := getDatabases(user, password, port)
	if err != nil {
		return dbinfo, fmt.Errorf("Unable to get databases info: %+v", err)
	}
	for _, db := range databases {
		engines, err := getEngines(user, password, port, db)
		if err != nil {
			return dbinfo, fmt.Errorf("Unable to get enigines info: %+v", err)
		}
		size, dataSize, err := getDatabaseSize(user, password, port, db)
		if err != nil {
			return dbinfo, fmt.Errorf("Unable to get databases size info: %+v", err)
		}
		if config.Config.CompressLogicalBackup {
			logicalSize = int64(float64(dataSize) * 0.6)
		} else {
			logicalSize = int64(float64(dataSize) * 0.8)
		}
		dbinfo[db] = &MySQLDatabase{engines, size, logicalSize}
	}
	return dbinfo, err
}
