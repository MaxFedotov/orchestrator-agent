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

package config

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/go-ini/ini"
	"github.com/openark/golib/log"
)

// Configuration makes for orchestrator-agent configuration input, which can be provided by user via JSON formatted file.
type Configuration struct {
	SnapshotMountPoint                 string            // The single, agreed-upon mountpoint for logical volume snapshots
	ContinuousPollSeconds              uint              // Poll interval for continuous operation
	ResubmitAgentIntervalMinutes       uint              // Poll interval for resubmitting this agent on orchestrator agents API
	CreateSnapshotCommand              string            // Command which creates a snapshot logical volume. It's a "do it yourself" implementation
	AvailableLocalSnapshotHostsCommand string            // Command which returns list of hosts (one host per line) with available snapshots in local datacenter
	AvailableSnapshotHostsCommand      string            // Command which returns list of hosts (one host per line) with available snapshots in any datacenter
	SnapshotVolumesFilter              string            // text pattern filtering agent logical volumes that are valid snapshots
	PreferStreaming                    bool              // If true, will prefer using backup streaming
	MySQLServiceStopCommand            string            // Command to stop mysql, e.g. /etc/init.d/mysql stop
	MySQLServiceStartCommand           string            // Command to start mysql, e.g. /etc/init.d/mysql start
	MySQLServiceRestartCommand         string            // Command to restart mysql, e.g. /etc/init.d/mysql restart
	MySQLServiceStatusCommand          string            // Command to check mysql status. Expects 0 return value when running, non-zero when not running, e.g. /etc/init.d/mysql status
	MySQLBackupDir                     string            // Path to directory on host where backup files will be stored
	SeedPort                           int               // Port used for transfering backup data
	ReceiveSeedDataCommand             string            // Accepts incoming data (e.g. tarball over netcat)
	SendSeedDataCommand                string            // Sends date to remote host (e.g. tarball via netcat)
	PostCopyCommand                    string            // command that is executed after seed is done and before MySQL starts
	MySQLClientCommand                 string            // the `mysql` command, including ny neccesary credentials, to apply relay logs. This would be a fully-privileged account entry. Example: "mysql -uroot -p123456" or "mysql --defaults-file=/root/.my.cnf"
	AgentsServer                       string            // HTTP address of the orchestrator agents server
	AgentsServerPort                   int               // HTTP port of the orchestrator agents server
	HTTPPort                           int               // HTTP port on which this service listens
	HTTPAuthUser                       string            // Username for HTTP Basic authentication (blank disables authentication)
	HTTPAuthPassword                   string            // Password for HTTP Basic authentication
	UseSSL                             bool              // If true, service will serve HTTPS only
	UseMutualTLS                       bool              // If true, service will serve HTTPS only
	SSLSkipVerify                      bool              // When using SSL, should we ignore SSL certification error
	SSLPrivateKeyFile                  string            // Name of SSL private key file, applies only when UseSSL = true
	SSLCertFile                        string            // Name of SSL certification file, applies only when UseSSL = true
	SSLCAFile                          string            // Name of SSL certificate authority file, applies only when UseSSL = true
	SSLValidOUs                        []string          // List of valid OUs that should be allowed for mutual TLS verification
	StatusEndpoint                     string            // The endpoint for the agent status check.  Defaults to /api/status
	StatusOUVerify                     bool              // If true, try to verify OUs when Mutual TLS is on.  Defaults to false
	StatusBadSeconds                   uint              // Report non-200 on a status check if we've failed to communicate with the main server in this number of seconds
	HTTPTimeoutSeconds                 int               // Number of idle seconds before HTTP GET request times out (when accessing orchestrator)
	ExecWithSudo                       bool              // If true, run os commands that need privileged access with sudo. Usually set when running agent with a non-privileged user
	CustomCommands                     map[string]string // Anything in this list of options will be exposed as an API callable options
	TokenHintFile                      string            // If defined, token will be stored in this file
	TokenHTTPHeader                    string            // If defined, name of HTTP header where token is presented (as alternative to query param)
	MySQLTopologyUser                  string            // Username used by orchestrator-agent to connect to MySQL
	MySQLTopologyPassword              string            // Password for MySQL user used by orchestrator-agent to connect to MySQL
	MySQLReplicationUser               string            // Username used to setup MySQL replication
	MySQLReplicationPassword           string            // Password for MySQLReplicationUser
	MySQLPort                          int               // Port on which mysqld is listening. Read from my.cnf
	MySQLDataDir                       string            // Location of MySQL datadir. Read from my.cnf
	MySQLErrorLog                      string            // Location of MySQL error log file. Read from my.cnf
	MySQLInnoDBLogDir                  string            // Location of ib_logfile. Read from my.cnf
	MySQLBackupOldDatadir              bool              // backup contents of targetHost datadir before starting seed process
	XtrabackupParallelThreads          int               // Number of threads Xtrabackup will use to copy multiple data files concurrently when creating a backup
	MyDumperParallelThreads            int               // Number of threads MyDumper\MyLoader will use for dumping and restoring data
	MyDumperRowsChunkSize              int               // Split table into chunks of this many rows. 0 - unlimited
	CompressLogicalBackup              bool              // Compress output mydumper/mysqldump files
	Datacenter                         string            // Datacenter, which host belongs to
	Cluster                            string            // Cluster, which host belongs to
	sync.RWMutex
}

// Config initialization
var Config = newConfiguration()
var confFiles = make(map[string]string)

func newConfiguration() *Configuration {
	return &Configuration{
		SnapshotMountPoint:                 "",
		ContinuousPollSeconds:              60,
		ResubmitAgentIntervalMinutes:       60,
		CreateSnapshotCommand:              "",
		AvailableLocalSnapshotHostsCommand: "",
		AvailableSnapshotHostsCommand:      "",
		SnapshotVolumesFilter:              "",
		PreferStreaming:                    true,
		MySQLServiceStopCommand:            "",
		MySQLServiceStartCommand:           "",
		MySQLServiceRestartCommand:         "",
		MySQLServiceStatusCommand:          "",
		MySQLBackupDir:                     "",
		SeedPort:                           21234,
		ReceiveSeedDataCommand:             "",
		SendSeedDataCommand:                "",
		PostCopyCommand:                    "",
		MySQLClientCommand:                 "mysql",
		AgentsServer:                       "",
		AgentsServerPort:                   3001,
		HTTPPort:                           3002,
		HTTPAuthUser:                       "",
		HTTPAuthPassword:                   "",
		UseSSL:                             false,
		UseMutualTLS:                       false,
		SSLSkipVerify:                      false,
		SSLPrivateKeyFile:                  "",
		SSLCertFile:                        "",
		SSLCAFile:                          "",
		SSLValidOUs:                        []string{},
		StatusEndpoint:                     "/api/status",
		StatusOUVerify:                     false,
		StatusBadSeconds:                   300,
		HTTPTimeoutSeconds:                 10,
		ExecWithSudo:                       false,
		CustomCommands:                     make(map[string]string),
		TokenHintFile:                      "",
		TokenHTTPHeader:                    "",
		MySQLTopologyUser:                  "",
		MySQLTopologyPassword:              "",
		MySQLReplicationUser:               "",
		MySQLReplicationPassword:           "",
		MySQLPort:                          3306,
		MySQLDataDir:                       "",
		MySQLErrorLog:                      "",
		MySQLInnoDBLogDir:                  "",
		MySQLBackupOldDatadir:              false,
		XtrabackupParallelThreads:          1,
		MyDumperParallelThreads:            1,
		MyDumperRowsChunkSize:              0,
		CompressLogicalBackup:              true,
		Datacenter:                         "",
		Cluster:                            "",
	}
}

// read reads configuration from given file, or silently skips if the file does not exist.
// If the file does exist, then it is expected to be in valid JSON format or the function bails out.
func readJSON(fileName string) (*Configuration, error) {
	Config.Lock()
	defer Config.Unlock()
	file, err := os.Open(fileName)
	if err == nil {
		decoder := json.NewDecoder(file)
		err := decoder.Decode(Config)
		if err == nil {
			log.Infof("Read config: %s", fileName)
			if _, ok := confFiles["Agent"]; !ok {
				confFiles["Agent"] = fileName
			}
		} else {
			log.Fatal("Cannot read config file:", fileName, err)
		}
	}
	return Config, err
}

// read reads configuration from given file, or silently skips if the file does not exist.
// If the file does exist, then it is expected to be in valid INI format or the function bails out.
func readINI(fileName string) (*Configuration, error) {
	Config.Lock()
	defer Config.Unlock()
	cfg, err := ini.LoadSources(ini.LoadOptions{AllowBooleanKeys: true}, fileName)
	if err != nil {
		return Config, err
	}
	sec := cfg.Section("mysqld")
	if !sec.HasKey("port") {
		log.Fatal("Cannot read port from config file:", fileName)
	}
	if !sec.HasKey("datadir") {
		log.Fatal("Cannot read datadir from config file:", fileName)
	}
	if sec.Haskey("log-error") {
		Config.MySQLErrorLog = sec.Key("log-error").String()
	} else {
		Config.MySQLErrorLog = sec.Key("log_error").String()
	}
	Config.MySQLPort, _ = sec.Key("port").Int()
	Config.MySQLDataDir = sec.Key("datadir").String()
	Config.MySQLInnoDBLogDir = sec.Key("innodb_log_group_home_dir").String()
	if _, ok := confFiles["MySQL"]; !ok {
		confFiles["MySQL"] = fileName
	}
	log.Infof("Read config: %s", fileName)
	return Config, err
}

// Read reads configuration from zero, either, some or all given files, in order of input.
// A file can override configuration provided in previous file.
func Read(fileNames []string, configtype string) *Configuration {
	switch configtype {
	case "appconfig":
		{
			for _, fileName := range fileNames {
				readJSON(fileName)
			}
		}
	case "mysqlconfig":
		{
			for _, fileName := range fileNames {
				readINI(fileName)
			}
		}
	}
	return Config
}

// ForceRead reads configuration from given file name or bails out if it fails
func ForceRead(fileName string) *Configuration {
	_, err := readJSON(fileName)
	if err != nil {
		log.Fatal("Cannot read config file:", fileName, err)
	}
	return Config
}

// WatchMySQLConf watches for changes in MySQL configuration files and rereads them in case of change
func WatchMySQLConf() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Error("Failed to start config watcher")
	}
	defer watcher.Close()
	done := make(chan bool)
	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if event.Op&fsnotify.Write == fsnotify.Write {
					log.Infof("MySQL config file %s changed. Reloading", event.Name)
					readINI(event.Name)
				}
			case err := <-watcher.Errors:
				log.Errorf("Unable to reload config file %s", err)
			}
		}
	}()

	if err := watcher.Add(confFiles["MySQL"]); err != nil {
		log.Errorf("Unable to add watcher for MySQL config file. Error: %s", err)
	}

	<-done
}
