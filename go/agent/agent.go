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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	nethttp "net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/github/orchestrator-agent/go/dbagent"
	"github.com/github/orchestrator-agent/go/helper/http"
	"github.com/github/orchestrator-agent/go/helper/mysql"
	"github.com/github/orchestrator-agent/go/helper/ssl"
	"github.com/github/orchestrator-agent/go/helper/token"
	"github.com/github/orchestrator-agent/go/osagent"
	"github.com/github/orchestrator-agent/go/seed"
	"github.com/go-martini/martini"
	"github.com/martini-contrib/auth"
	"github.com/martini-contrib/gzip"
	"github.com/martini-contrib/render"
	log "github.com/sirupsen/logrus"
)

type Agent struct {
	Info                  *Info
	AvailiableSeedMethods map[seed.Method]*seed.MethodOpts
	MySQLDatadir          string
	Config                *Config
	ConfigFileName        string
	HTTPClient            *nethttp.Client
	MySQLClient           *mysql.MySQLClient
	LastTalkback          time.Time
	Logger                *log.Entry
	StatusChan            chan *seed.SeedStageState
	SeedMethods           map[seed.Method]seed.Plugin
	ActiveSeed            *ActiveSeed
	sync.RWMutex
}

type Info struct {
	Hostname  string
	Port      int
	Token     string
	MySQLPort int
}

type ActiveSeed struct {
	SeedID        int64
	SeedStatus    seed.Status
	Method        seed.Method
	Side          seed.Side
	Stage         seed.Stage
	StagesDetails map[seed.Stage]*seed.SeedStageState
}

type Data struct {
	LocalSnapshotsHosts   []string                 // AvailableLocalSnapshots in Orchestrator
	SnaphostHosts         []string                 // AvailableSnapshots in Orchestrator
	LogicalVolumes        []*osagent.LogicalVolume // pass by reference ??
	MountPoint            *osagent.Mount           // pass by reference ??
	BackupDir             string
	BackupDirDiskFree     int64
	BackupDirDiskUsed     int64
	MySQLRunning          bool
	MySQLDatadir          string
	MySQLDatadirDiskUsed  int64
	MySQLDatadirDiskFree  int64
	MySQLDatabases        map[string]*dbagent.MySQLDatabase
	AvailiableSeedMethods map[seed.Method]*seed.MethodOpts
}

// New creates new instance of orchestrator-agent
func New(configFilename string, logger *log.Entry) *Agent {
	agent := &Agent{
		ConfigFileName: configFilename,
		Logger:         logger,
	}
	return agent
}

// LoadConfig load and parse orchestrator-agent configuration
func (agent *Agent) LoadConfig() error {
	agent.Lock()
	defer agent.Unlock()

	return agent.parseConfig()
}

func (agent *Agent) parseConfig() error {
	cfg, err := ReadConfig(agent.ConfigFileName)
	if err != nil {
		return err
	}
	if cfg.Common.UseSSL {
		if len(cfg.Common.SSLCertFile) == 0 {
			return fmt.Errorf("use-ssl is true but ssl-cert-file is not specified")
		}
		if len(cfg.Common.SSLPrivateKeyFile) == 0 {
			return fmt.Errorf("use-ssl is true but ssl-private-key-file is not specified")
		}
	}
	if len(cfg.Common.BackupDir) == 0 {
		return fmt.Errorf("backup-dir not specified")
	}
	if len(cfg.Orchestrator.URL) == 0 {
		return fmt.Errorf("orchestrator url not specified")
	}
	if len(cfg.Mysql.SeedUser) == 0 {
		return fmt.Errorf("mysql seed-user not specified")
	}
	if len(cfg.Mysql.SeedPassword) == 0 {
		return fmt.Errorf("mysql seed-password not specified")
	}
	if cfg.LVM.Enabled {
		if len(cfg.LVM.CreateSnapshotCommand) == 0 {
			return fmt.Errorf("lvm is enabled but create-snapshot-command is not specified")
		}
		if len(cfg.LVM.AvailableLocalSnapshotHostsCommand) == 0 {
			return fmt.Errorf("lvm is enabled but available-local-snapshot-hosts-command is not specified")
		}
		if len(cfg.LVM.AvailableSnapshotHostsCommand) == 0 {
			return fmt.Errorf("lvm is enabled but available-snapshot-hosts-command is not specified")
		}
		if len(cfg.LVM.SnapshotVolumesFilter) == 0 {
			return fmt.Errorf("lvm is enabled but snapshot-volumes-filter is not specified")
		}
	}
	if len(cfg.Logging.File) != 0 {
		logFile, err := os.OpenFile(cfg.Logging.File, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0640)
		if err != nil {
			return fmt.Errorf("Unable to open log file")
		}
		mw := io.MultiWriter(os.Stdout, logFile)
		log.SetOutput(mw)
	}
	switch strings.ToLower(cfg.Logging.Level) {
	case "debug":
		{
			log.SetLevel(log.DebugLevel)
		}
	case "info":
		{
			log.SetLevel(log.InfoLevel)
		}
	case "error":
		{
			log.SetLevel(log.ErrorLevel)
		}
	case "warn":
		{
			log.SetLevel(log.WarnLevel)
		}
	}
	agent.Config = cfg
	return nil
}

// Start agent
func (agent *Agent) Start() error {
	agent.Lock()
	defer agent.Unlock()

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("Unable to get hostname: %+v", err)
	}
	agent.Info = &Info{
		Hostname:  hostname,
		Port:      agent.Config.Common.Port,
		MySQLPort: agent.Config.Mysql.Port,
	}
	agentToken, err := token.NewToken()
	if err != nil {
		return fmt.Errorf("Unable to generate token for agent: %+v", err)
	}
	agent.Info.Token = agentToken.Hash
	if agent.Config.Common.TokenHintFile != "" {
		agent.Logger.WithField("TokenHintFile", agent.Config.Common.TokenHintFile).Debug("Writing token to file")
		if err := ioutil.WriteFile(agent.Config.Common.TokenHintFile, []byte(agent.Info.Token), 0644); err != nil {
			agent.Logger.WithField("error", err).Error("Unable to create token hint file")
		}
	}

	agent.HTTPClient = http.InitHTTPClient(agent.Config.Common.HTTPTimeout, agent.Config.Common.SSLSkipVerify, agent.Config.Common.SSLCAFile, agent.Config.Common.UseMutualTLS, agent.Config.Common.SSLCertFile, agent.Config.Common.SSLPrivateKeyFile, agent.Logger)

	agent.MySQLClient, err = dbagent.NewMySQLClient(agent.Config.Mysql.SeedUser, agent.Config.Mysql.SeedPassword, agent.Config.Mysql.Port)
	if err != nil {
		return fmt.Errorf("Unable to connect to MySQL: %+v", err)
	}
	agent.MySQLDatadir, err = dbagent.GetMySQLDatadir(agent.MySQLClient)
	if err != nil {
		return fmt.Errorf("Unable to get MySQL datadir path: %+v", err)
	}
	if err := osagent.CheckPermissionsOnFolder(agent.Config.Common.BackupDir, agent.Config.Common.ExecWithSudo); err != nil {
		return fmt.Errorf("Agent don't have permissions on backup directory %s", agent.Config.Common.BackupDir)
	}
	if err := osagent.CheckPermissionsOnFolder(agent.MySQLDatadir, agent.Config.Common.ExecWithSudo); err != nil {
		return fmt.Errorf("Agent don't have permissions on MySQL data directory %s", agent.MySQLDatadir)
	}
	agent.StatusChan = make(chan *seed.SeedStageState)
	seedBaseConfig := seed.Base{
		MySQLClient:   agent.MySQLClient,
		MySQLPort:     agent.Config.Mysql.Port,
		SeedUser:      agent.Config.Mysql.SeedUser,
		SeedPassword:  agent.Config.Mysql.SeedPassword,
		ExecWithSudo:  agent.Config.Common.ExecWithSudo,
		SeedPort:      agent.Config.Common.SeedPort,
		UseSSL:        agent.Config.Common.UseSSL,
		SSLSkipVerify: agent.Config.Common.SSLSkipVerify,
		SSLCertFile:   agent.Config.Common.SSLCertFile,
		SSLCAFile:     agent.Config.Common.SSLCAFile,
		BackupDir:     agent.Config.Common.BackupDir,
		StatusChan:    agent.StatusChan,
	}
	seedMethods := make(map[seed.Method]seed.Plugin)
	availiableSeedMethods := make(map[seed.Method]*seed.MethodOpts)
	agent.ActiveSeed = &ActiveSeed{}
	if agent.Config.LVM.Enabled {
		lvm, lvmOpts, err := seed.New(
			seed.LVM,
			&seedBaseConfig,
			log.WithFields(log.Fields{"prefix": "LVM"}),
			agent.Config.LVM,
		)
		if err != nil {
			agent.Logger.WithField("error", err).Fatal("Unable to use LVM seed method")
		} else {
			seedMethods[seed.LVM] = lvm
			availiableSeedMethods[seed.LVM] = lvmOpts
			agent.Logger.Info("LVM seed method initialized")
		}
	}
	if agent.Config.Xtrabackup.Enabled {
		xtrabackup, xtrabackupOpts, err := seed.New(
			seed.Xtrabackup,
			&seedBaseConfig,
			log.WithFields(log.Fields{"prefix": "XTRABACKUP"}),
			agent.Config.Xtrabackup,
		)
		if err != nil {
			agent.Logger.WithField("error", err).Fatal("Unable to use Xtrabackup seed method")
		} else {
			seedMethods[seed.Xtrabackup] = xtrabackup
			availiableSeedMethods[seed.Xtrabackup] = xtrabackupOpts
			agent.Logger.Info("Xtrabackup seed method initialized")
		}
	}
	if agent.Config.ClonePlugin.Enabled {
		clonePlugin, clonePluginOpts, err := seed.New(
			seed.ClonePlugin,
			&seedBaseConfig,
			log.WithFields(log.Fields{"prefix": "CLONE PLUGIN"}),
			agent.Config.ClonePlugin,
		)
		if err != nil {
			agent.Logger.WithField("error", err).Fatal("Unable to use Clone plugin seed method")
		} else {
			seedMethods[seed.ClonePlugin] = clonePlugin
			availiableSeedMethods[seed.ClonePlugin] = clonePluginOpts
			agent.Logger.Info("Clone plugin seed method initialized")
		}
	}
	if agent.Config.MysqlDump.Enabled {
		mysqldump, mysqldumpOpts, err := seed.New(
			seed.Mysqldump,
			&seedBaseConfig,
			log.WithFields(log.Fields{"prefix": "MYSQLDUMP"}),
			agent.Config.MysqlDump,
		)
		if err != nil {
			agent.Logger.WithField("error", err).Fatal("Unable to use Mysqldump seed method")
		} else {
			seedMethods[seed.Mysqldump] = mysqldump
			availiableSeedMethods[seed.Mysqldump] = mysqldumpOpts
			agent.Logger.Info("Mysqldump seed method initialized")
		}
	}
	if agent.Config.Mydumper.Enabled {
		mydumper, mydumperOpts, err := seed.New(
			seed.Mydumper,
			&seedBaseConfig,
			log.WithFields(log.Fields{"prefix": "MYDUMPER"}),
			agent.Config.Mydumper,
		)
		if err != nil {
			agent.Logger.WithField("error", err).Fatal("Unable to use Mydumper seed method")
		} else {
			seedMethods[seed.Mydumper] = mydumper
			availiableSeedMethods[seed.Mydumper] = mydumperOpts
			agent.Logger.Info("Mydumper seed method initialized")
		}
	}
	agent.AvailiableSeedMethods = availiableSeedMethods
	agent.SeedMethods = seedMethods
	go agent.ContinuousOperation()
	go agent.ServeHTTP()
	go agent.UpdateSeedStatus()
	return nil
}

func (agent *Agent) UpdateSeedStatus() {
	for {
		select {
		case seedStatus := <-agent.StatusChan:
			seedStatus.SeedID = agent.ActiveSeed.SeedID
			agent.Lock()
			agent.ActiveSeed.StagesDetails[seedStatus.Stage] = seedStatus
			// Last stage is Cleanup, so if it is Completed - seed is Completed
			if seedStatus.Stage == seed.Cleanup && seedStatus.Status == seed.Completed {
				agent.ActiveSeed.SeedStatus = seed.Completed
			}
			agent.Unlock()
		}
	}
}

func (agent *Agent) ServeHTTP() {
	m := http.NewMartini()
	if agent.Config.Common.HTTPAuthUser != "" {
		m.Use(auth.Basic(agent.Config.Common.HTTPAuthUser, agent.Config.Common.HTTPAuthPassword))
	}

	m.Map(agent)
	m.Use(gzip.All())
	// Render html templates from templates directory
	m.Use(render.Renderer(render.Options{
		Directory:       "resources",
		Layout:          "templates/layout",
		HTMLContentType: "text/html",
	}))
	m.Use(martini.Static("resources/public"))
	if agent.Config.Common.UseMutualTLS {
		m.Use(ssl.VerifyOUs(agent.Config.Common.SSLValidOUs, agent.Config.Common.StatusEndpoint, agent.Config.Common.StatusOUVerify, agent.Logger))
	}

	agent.Logger.WithField("port", agent.Config.Common.Port).Info("Starting HTTP Server")

	API.RegisterRequests(m)

	listenAddress := fmt.Sprintf(":%d", agent.Config.Common.Port)

	// Serve
	if agent.Config.Common.UseSSL {
		log.Info("Starting HTTPS listener")
		tlsConfig, err := ssl.NewTLSConfig(agent.Config.Common.SSLCAFile, agent.Config.Common.UseMutualTLS, agent.Logger)
		if err != nil {
			log.Fatal(err)
		}
		if err = ssl.AppendKeyPair(tlsConfig, agent.Config.Common.SSLCertFile, agent.Config.Common.SSLPrivateKeyFile); err != nil {
			log.Fatal(err)
		}
		if err = ssl.ListenAndServeTLS(listenAddress, m, tlsConfig); err != nil {
			log.Fatal(err)
		}
	} else {
		agent.Logger.Info("Starting HTTP listener")
		if err := nethttp.ListenAndServe(listenAddress, m); err != nil {
			log.Fatal(err)
		}
	}
	agent.Logger.Info("Web server started")
}

//RegisterAgent registers agent on Orchestrator
func (agent *Agent) RegisterAgent() {
	url := fmt.Sprintf("%s:%d/api/register-agent", agent.Config.Orchestrator.URL, agent.Config.Orchestrator.AgentsPort)
	payload, err := json.Marshal(agent.Info)
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to marshall agent info")
	}
	agent.Logger.WithFields(log.Fields{"url": url}).Debug("Submiting agent to Orchestrator")

	response, err := agent.HTTPClient.Post(url, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to submit agent to Orchestrator")
	} else {
		defer response.Body.Close()
		agent.Logger.WithField("response", response).Debug("Agent added to Orchestrator")
	}
}

// PingServer checks connectivity back to the orchestrator server
func (agent *Agent) PingServer(url string) error {
	response, err := agent.HTTPClient.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return nil
}

// ContinuousOperation starts an asynchronuous infinite operation process where:
// - agent is submitted into orchestrator
func (agent *Agent) ContinuousOperation() {
	agent.Logger.Info("Starting continuous submit operation")
	tick := time.Tick(agent.Config.Common.PollInterval.Value())
	resubmitTick := time.Tick(agent.Config.Common.ResubmitAgentInterval.Value())
	url := fmt.Sprintf("%s:%d/api/agent-ping", agent.Config.Orchestrator.URL, agent.Config.Orchestrator.AgentsPort)

	agent.RegisterAgent()
	for range tick {
		// Do stuff
		if err := agent.PingServer(url); err != nil {
			agent.Logger.WithField("url", url).Warn("Failed to ping orchestrator server")
		} else {
			agent.LastTalkback = time.Now()
		}

		// See if we should also forget instances/agents (lower frequency)
		select {
		case <-resubmitTick:
			agent.RegisterAgent()
		default:
		}
	}
}

func (agent *Agent) GetMySQLErrorLog() ([]string, error) {
	var mySQLErrorLogTail []string
	mySQLLogFile, err := dbagent.GetMySQLLogFile(agent.MySQLClient)
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to get MySQL log file info")
	} else {
		mySQLErrorLogTail, err = osagent.GetMySQLErrorLogTail(mySQLLogFile, agent.Config.Common.ExecWithSudo)
		if err != nil {
			agent.Logger.WithField("error", err).Error("Unable to read MySQL log file")
		}
	}
	return mySQLErrorLogTail, err
}

// GetAgentInfo return system and MySQL information of the agent host
func (agent *Agent) GetAgentData() *Data {
	agent.Lock()
	defer agent.Unlock()

	var err error
	agentData := &Data{}
	agentData.LocalSnapshotsHosts, err = osagent.GetSnapshotHosts(agent.Config.LVM.AvailableLocalSnapshotHostsCommand, agent.Config.Common.ExecWithSudo)
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to get local snapshot hosts")
	}
	agentData.SnaphostHosts, err = osagent.GetSnapshotHosts(agent.Config.LVM.AvailableSnapshotHostsCommand, agent.Config.Common.ExecWithSudo)
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to get snapshot hosts")
	}
	agentData.LogicalVolumes, err = osagent.GetLogicalVolumes("", agent.Config.LVM.SnapshotVolumesFilter, agent.Config.Common.ExecWithSudo)
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to get logical volumes info")
	}
	agentData.MountPoint, err = osagent.GetMount(agent.Config.Common.BackupDir, agent.Config.Common.ExecWithSudo)
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to get snapshot mount point info")
	}
	agentData.BackupDir = agent.Config.Common.BackupDir
	agentData.AvailiableSeedMethods = agent.AvailiableSeedMethods
	agentData.BackupDirDiskFree, err = osagent.GetFSStatistics(agent.Config.Common.BackupDir, osagent.Free)
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to get backup directory free space info")
	}
	agentData.BackupDirDiskUsed, err = osagent.GetDiskUsage(agent.Config.Common.BackupDir, agent.Config.Common.ExecWithSudo)
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to get MySQL backup directory used space info")
	}
	agentData.MySQLRunning, err = osagent.MySQLRunning(agent.Config.Common.ExecWithSudo)
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to get information about MySQL status (running/stopped)")
	}
	agentData.MySQLDatadir = agent.MySQLDatadir
	agentData.MySQLDatadirDiskUsed, err = osagent.GetDiskUsage(agentData.MySQLDatadir, agent.Config.Common.ExecWithSudo)
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to get MySQL datadir used space info")
	}
	agentData.MySQLDatadirDiskFree, err = osagent.GetFSStatistics(agentData.MySQLDatadir, osagent.Free)
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to get MySQL datadir free space info")
	}
	agentData.MySQLDatabases, err = dbagent.GetMySQLDatabases(agent.MySQLClient)
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to get MySQL databases info")
	}
	return agentData
}
