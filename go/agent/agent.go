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
	"io"
	"io/ioutil"
	nethttp "net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/github/orchestrator-agent/go/dbagent"
	"github.com/github/orchestrator-agent/go/helper/http"
	"github.com/github/orchestrator-agent/go/helper/ssl"
	"github.com/github/orchestrator-agent/go/helper/token"
	"github.com/github/orchestrator-agent/go/seed"
	"github.com/go-martini/martini"
	"github.com/martini-contrib/auth"
	"github.com/martini-contrib/gzip"
	"github.com/martini-contrib/render"
	log "github.com/sirupsen/logrus"
)

type Agent struct {
	Params         *AgentParams
	Info           *AgentInfo
	Config         *Config
	SeedMethods    map[string]*seed.SeedMethod
	ConfigFileName string
	HTTPClient     *nethttp.Client
	MySQLClient    *dbagent.MySQLClient
	LastTalkback   time.Time
	Logger         *log.Entry
	sync.RWMutex
}

type AgentParams struct {
	Hostname string
	Port     int
	Token    string
}

type AgentInfo struct {
	AvailableLocalSnapshots []string
	AvailableSnapshots      []string
	LogicalVolumes          []LogicalVolume
	MountPoint              Mount
	MySQLRunning            bool
	MySQLPort               int
	MySQLDatadir            string
	MySQLDatadirSize        int64
	MySQLDatadirDiskFree    int64
	BackupDir               string
	BackupDirDiskFree       int64
	MySQLInnoDBLogSize      int64 // do we need it?
	MySQLErrorLogTail       []string
	MySQLDatabases          map[string]*dbagent.MySQLDatabase
}

// LogicalVolume describes an LVM volume
type LogicalVolume struct {
	Name            string
	GroupName       string
	Path            string
	IsSnapshot      bool
	SnapshotPercent float64
	SnapshotDate    time.Time
}

// Mount describes a file system mount point
type Mount struct {
	Path       string
	Device     string
	LVPath     string
	FileSystem string
	IsMounted  bool
	DiskUsage  int64
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
	if len(cfg.Mysql.ReplicationUser) == 0 {
		return fmt.Errorf("mysql replication-user not specified")
	}
	if len(cfg.Mysql.ReplicationPassword) == 0 {
		return fmt.Errorf("mysql replication-password not specified")
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
	if cfg.Common.TokenHintFile != "" {
		agent.Logger.WithField("TokenHintFile", cfg.Common.TokenHintFile).Debug("Writing token to file")
		err := ioutil.WriteFile(cfg.Common.TokenHintFile, []byte(agent.Params.Token), 0644)
		agent.Logger.WithField("error", err).Error("Unable to create token hint file")
	}
	agent.Config = cfg
	return nil
}

// Start agent
func (agent *Agent) Start() error {
	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("Unable to get hostname: %+v", err)
	}
	agent.Params = &AgentParams{
		Hostname: hostname,
		Port:     agent.Config.Common.Port,
		Token:    token.ProcessToken.Hash,
	}
	agent.HTTPClient = http.InitHTTPClient(agent.Config.Common.HTTPTimeout, agent.Config.Common.SSLSkipVerify, agent.Config.Common.SSLCAFile, agent.Config.Common.UseMutualTLS, agent.Config.Common.SSLCertFile, agent.Config.Common.SSLPrivateKeyFile, agent.Logger)

	agent.MySQLClient, err = dbagent.NewMySQLClient(agent.Config.Mysql.SeedUser, agent.Config.Mysql.SeedPassword, agent.Config.Mysql.Port)
	if err != nil {
		return fmt.Errorf("Unable to connect to MySQL: %+v", err)
	}
	// here goes each of seedmethods initializations

	go agent.ContinuousOperation()
	go agent.ServeHTTP()
	return nil
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
		m.Use(ssl.VerifyOUs(agent.Config.Common.SSLValidOUs, agent.Config.Common.StatusEndpoint, agent.Config.Common.StatusOUVerify))
	}

	agent.Logger.WithField("port", agent.Config.Common.Port).Info("Starting HTTP Server")

	API.RegisterRequests(m)

	listenAddress := fmt.Sprintf(":%d", agent.Config.Common.Port)

	// Serve
	if agent.Config.Common.UseSSL {
		log.Info("Starting HTTPS listener")
		tlsConfig, err := ssl.NewTLSConfig(agent.Config.Common.SSLCAFile, agent.Config.Common.UseMutualTLS)
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

//SubmitAgent registers agent on Orchestrator
func (agent *Agent) SubmitAgent() {
	url := fmt.Sprintf("%s:%d/api/submit-agent/%s/%d/%s", agent.Config.Orchestrator.URL, agent.Config.Orchestrator.AgentsPort, agent.Params.Hostname, agent.Params.Port, agent.Params.Token)
	agent.Logger.WithField("url", url).Debug("Submiting agent to Orchestrator")

	response, err := agent.HTTPClient.Get(url)
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

	agent.SubmitAgent()
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
			agent.SubmitAgent()
		default:
		}
	}
}
