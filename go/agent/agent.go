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
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"plugin"
	"strings"
	"time"

	"github.com/github/orchestrator-agent/go/config"
	"github.com/github/orchestrator-agent/go/ssl"
	"github.com/openark/golib/log"
)

// OrchAgent initializes new orchestrator-agent instance
var OrchestratorAgent = *initializeAgent()

// Agent represents basic agent data and methods
type Agent struct {
	Hostname      string
	Port          uint
	Token         string
	BackupPlugins map[string]BackupPlugin
}

// AgentInfo presents the data of an agent
type AgentInfo struct {
	AvailableLocalSnapshots []string
	AvailableSnapshots      []string
	LogicalVolumes          []LogicalVolume
	MountPoint              Mount
	MySQLRunning            bool
	MySQLPort               int64
	MySQLDatadir            string
	MySQLDatadirSize        int64
	MySQLDatadirDiskFree    int64
	MySQLBackupdir          string
	MySQLBackupdirDiskFree  int64
	MySQLInnoDBLogSize      int64
	MySQLErrorLogTail       []string
	MySQLDatabases          map[string]*MySQLDatabase
	AvailiableSeedMethods   []string
}

// MySQLDatabase desctibes a MySQL database
type MySQLDatabase struct {
	Engines      []string
	PhysicalSize int64
	LogicalSize  int64
}

// LogicalVolume describes an LVM volume
type LogicalVolume struct {
	Name            string
	GroupName       string
	Path            string
	IsSnapshot      bool
	SnapshotPercent float64
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

type BackupPlugin interface {
	Backup()
	Restore()
	GetMetadata()
	SupportedEngines() []string
	IsAvailiable() bool
}

var (
	httpTimeout  = time.Duration(time.Duration(config.Config.HTTPTimeoutSeconds) * time.Second)
	httpClient   = &http.Client{}
	LastTalkback time.Time
)

func dialTimeout(network, addr string) (net.Conn, error) {
	return net.DialTimeout(network, addr, httpTimeout)
}

// httpGet is a convenience method for getting http response from URL, optionaly skipping SSL cert verification
func httpGet(url string) (resp *http.Response, err error) {
	tlsConfig, _ := buildTLS()
	httpTransport := &http.Transport{
		TLSClientConfig: tlsConfig,
		Dial:            dialTimeout,
		ResponseHeaderTimeout: httpTimeout,
	}
	httpClient.Transport = httpTransport
	return httpClient.Get(url)
}

func buildTLS() (*tls.Config, error) {
	tlsConfig, err := ssl.NewTLSConfig(config.Config.SSLCAFile, config.Config.UseMutualTLS)
	if err != nil {
		return tlsConfig, log.Errore(err)
	}
	_ = ssl.AppendKeyPair(tlsConfig, config.Config.SSLCertFile, config.Config.SSLPrivateKeyFile)
	tlsConfig.InsecureSkipVerify = config.Config.SSLSkipVerify
	return tlsConfig, nil
}

// ContinuousOperation starts an asynchronuous infinite operation process where:
// - agent is submitted into orchestrator
func ContinuousOperation() {
	log.Infof("Starting continuous operation")
	tick := time.Tick(time.Duration(config.Config.ContinuousPollSeconds) * time.Second)
	resubmitTick := time.Tick(time.Duration(config.Config.ResubmitAgentIntervalMinutes) * time.Minute)

	OrchestratorAgent.submitAgent()
	for range tick {
		// Do stuff
		if err := OrchestratorAgent.pingServer(); err != nil {
			log.Warning("Failed to ping orchestrator server")
		} else {
			LastTalkback = time.Now()
		}

		// See if we should also forget instances/agents (lower frequency)
		select {
		case <-resubmitTick:
			OrchestratorAgent.submitAgent()
		default:
		}
	}
}

func initializeAgent() *Agent {
	agent := Agent{}
	workingDir, err := os.Getwd()
	if err != nil {
		log.Fatal("Unable to get working directory")
	}
	agent.Hostname, _ = os.Hostname()
	agent.Port = config.Config.HTTPPort
	agent.Token = ProcessToken.Hash
	agent.BackupPlugins, _ = InitilizePlugins(filepath.Join(workingDir, "plugins"))
	//agent.BackupPlugins["mysqldump"].Backup()
	//agent.BackupPlugins["mydumper"].Backup()
	return &agent
}

func (agent Agent) submitAgent() error {
	url := fmt.Sprintf("%s/api/submit-agent/%s/%d/%s", config.Config.AgentsServer+config.Config.AgentsServerPort, agent.Hostname, agent.Port, agent.Token)
	log.Debugf("Submitting this agent: %s", url)

	response, err := httpGet(url)
	if err != nil {
		return log.Errore(err)
	}
	defer response.Body.Close()

	log.Debugf("response: %+v", response)
	return err
}

// Just check connectivity back to the server.  This just calls an endpoint that returns 200 OK
func (agent Agent) pingServer() error {
	url := fmt.Sprintf("%s/api/agent-ping", config.Config.AgentsServer+config.Config.AgentsServerPort)
	response, err := httpGet(url)
	if err != nil {
		return log.Errore(err)
	}
	defer response.Body.Close()
	return nil
}

func InitilizePlugins(pluginDir string) (map[string]BackupPlugin, error) {
	plugins := make(map[string]BackupPlugin)
	pluginFiles, err := filepath.Glob(filepath.Join(pluginDir, "*.so"))
	if err != nil {
		return nil, log.Fatalf("Unable to get plugins from %s", pluginDir)
	}
	if pluginFiles == nil {
		log.Errorf("No plugins found in %s", pluginDir)
	}
	for _, file := range pluginFiles {
		var newBackupPlugin BackupPlugin
		pluginName := strings.TrimSuffix(filepath.Base(file), filepath.Ext(filepath.Base(file)))
		plug, err := plugin.Open(file)
		if err != nil {
			log.Errorf("Unable to load plugin %s from %s: %s", filepath.Base(file), pluginDir, err)
		}
		loadedPlugin, err := plug.Lookup("BackupPlugin")
		if err != nil {
			log.Errorf("Error loading plugin %s from %s: %s", filepath.Base(file), pluginDir, err)
		}
		newBackupPlugin, ok := loadedPlugin.(BackupPlugin)
		if !ok {
			log.Errorf("Error loading plugin %s from %s", filepath.Base(file), pluginDir)
		}
		plugins[pluginName] = newBackupPlugin
		log.Infof("Succesfully loaded %s plugin", pluginName)
	}
	return plugins, err
}

// GetAgentInfo returns information about agent to orchestrator
func (agent Agent) GetAgentInfo() AgentInfo {
	var err error
	agentInfo := AgentInfo{}

	config.Config.Lock()
	defer config.Config.Unlock()

	agentInfo.AvailableLocalSnapshots, err = availableSnapshots(true)
	if err != nil {
		log.Errore(err)
	}
	agentInfo.AvailableSnapshots, err = availableSnapshots(false)
	if err != nil {
		log.Errore(err)
	}
	agentInfo.LogicalVolumes, err = logicalVolumes("", config.Config.SnapshotVolumesFilter)
	if err != nil {
		log.Errore(err)
	}
	agentInfo.MountPoint = getMount(config.Config.SnapshotMountPoint)
	agentInfo.MySQLRunning = mySQLRunning()
	agentInfo.MySQLPort = config.Config.MySQLPort
	agentInfo.MySQLDatadir = config.Config.MySQLDataDir
	agentInfo.MySQLDatadirSize, err = getDirectorySize(config.Config.MySQLDataDir)
	if err != nil {
		log.Errore(err)
	}
	agentInfo.MySQLDatadirDiskFree, err = getDiskStatistics(config.Config.MySQLDataDir, "free")
	if err != nil {
		log.Errore(err)
	}
	agentInfo.MySQLBackupdir = config.Config.MySQLBackupDir
	agentInfo.MySQLBackupdirDiskFree, err = getDiskStatistics(config.Config.MySQLBackupDir, "free")
	if err != nil {
		log.Errore(err)
	}
	agentInfo.MySQLInnoDBLogSize, err = getInnoDBLogSize(config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort)
	if err != nil {
		log.Errore(fmt.Errorf("Unable to get innoDB Log size info: %+v", err))
	}
	agentInfo.MySQLErrorLogTail, err = mySQLErrorLogTail(config.Config.MySQLErrorLog)
	agentInfo.MySQLDatabases, err = getMySQLDatabases(config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort)
	if err != nil {
		log.Errore(err)
	}
	for plugin := range agent.BackupPlugins {
		if agent.BackupPlugins[plugin].IsAvailiable() {
			agentInfo.AvailiableSeedMethods = append(agentInfo.AvailiableSeedMethods, plugin)
		}
	}
	return agentInfo
}
