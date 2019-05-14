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
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"plugin"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/github/orchestrator-agent/go/config"
	"github.com/github/orchestrator-agent/go/helper/structs"
	"github.com/github/orchestrator-agent/go/ssl"
	"github.com/openark/golib/log"
)

var (
	httpTimeout  = time.Duration(time.Duration(config.Config.HTTPTimeoutSeconds) * time.Second)
	httpClient   = &http.Client{}
	LastTalkback time.Time
	Agent        agent
)

// Agent represents basic agent data and methods
type agent struct {
	Hostname      string
	Port          int
	Token         string
	Datacenter    string
	Cluster       string
	BackupPlugins map[string]BackupPlugin
	Params        structs.AgentParams
}

// AgentInfo presents the data of an agent
type AgentInfo struct {
	AvailableLocalSnapshots      []string
	AvailableSnapshots           []string
	LogicalVolumes               []LogicalVolume
	MountPoint                   Mount
	MySQLRunning                 bool
	MySQLPort                    int
	MySQLDatadir                 string
	MySQLDatadirSize             int64
	MySQLDatadirDiskFree         int64
	BackupDir                    string
	BackupDirDiskFree            int64
	MySQLInnoDBLogSize           int64
	MySQLErrorLogTail            []string
	MySQLDatabases               map[string]*MySQLDatabase
	AvailiableSeedMethodsBackup  map[string]*SeedMethod
	AvailiableSeedMethodsRestore map[string]*SeedMethod
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

// SeedMethod describes capabilities of a seed method
type SeedMethod struct {
	SupportedEngines         []string
	SupportDatabaseSelection bool
	SupportPhysicalBackup    bool
}

// BackupPlugin is an interface describing backup plugin fuctions
type BackupPlugin interface {
	Prepare(params structs.AgentParams, hostType string) error
	Backup(params structs.AgentParams, databases []string, errs chan error) io.Reader
	Receive(params structs.AgentParams, data io.Reader) error
	Restore(params structs.AgentParams) error
	GetMetadata(params structs.AgentParams) (structs.BackupMetadata, error)
	Cleanup(params structs.AgentParams, hostType string) error
	SupportedEngines() []string
	IsAvailiableBackup() bool
	IsAvailiableRestore() bool
	SupportDatabaseSelection() bool
}

func dialTimeout(network, addr string) (net.Conn, error) {
	return net.DialTimeout(network, addr, httpTimeout)
}

// httpGet is a convenience method for getting http response from URL, optionaly skipping SSL cert verification
func httpGet(url string) (resp *http.Response, err error) {
	tlsConfig, _ := buildTLS()
	httpTransport := &http.Transport{
		TLSClientConfig:       tlsConfig,
		Dial:                  dialTimeout,
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

	Agent.submitAgent()
	for range tick {
		// Do stuff
		if err := Agent.pingServer(); err != nil {
			log.Warning("Failed to ping orchestrator server")
		} else {
			LastTalkback = time.Now()
		}

		// See if we should also forget instances/agents (lower frequency)
		select {
		case <-resubmitTick:
			Agent.submitAgent()
		default:
		}
	}
}

//InitializeAgent is used to setup new agent
func InitializeAgent() {
	config.Config.Lock()
	defer config.Config.Unlock()
	workingDir, err := os.Getwd()
	if err != nil {
		log.Fatal("Unable to get working directory")
	}
	Agent.Hostname, _ = os.Hostname()
	Agent.Port = config.Config.HTTPPort
	Agent.Token = ProcessToken.Hash
	Agent.Cluster = config.Config.Cluster
	Agent.Datacenter = config.Config.Datacenter
	Agent.BackupPlugins = initilizePlugins(filepath.Join(workingDir, "plugins"))
	Agent.Params.MysqlUser = config.Config.MySQLTopologyUser
	Agent.Params.MysqlPassword = config.Config.MySQLTopologyPassword
	Agent.Params.MysqlPort = config.Config.MySQLPort
	Agent.Params.MysqlDatadir = config.Config.MySQLDataDir
	Agent.Params.BackupFolder = config.Config.BackupDir
	Agent.Params.InnoDBLogDir = config.Config.MySQLInnoDBLogDir
}

func (Agent agent) Cleanup(seedID string, seedMethod string, hostType string) error {
	if ok := Agent.isPluginLoaded(seedMethod); !ok {
		return log.Errorf("Unable to start cleanup process, plugin %s not loaded", seedMethod)
	}
	return log.Errore(Agent.BackupPlugins[seedMethod].Cleanup(Agent.Params, hostType))
}

func (Agent agent) GetMetadata(seedID string, seedMethod string) (metadata structs.BackupMetadata, err error) {
	metadata = structs.BackupMetadata{}
	if ok := Agent.isPluginLoaded(seedMethod); !ok {
		return metadata, log.Errorf("Unable to get metadata, plugin %s not loaded", seedMethod)
	}
	metadata, err = Agent.BackupPlugins[seedMethod].GetMetadata(Agent.Params)
	metadata.MasterUser = config.Config.MySQLReplicationUser
	metadata.MasterPassword = config.Config.MySQLReplicationPassword
	return metadata, log.Errore(err)
}

func (Agent agent) Prepare(seedID string, seedMethod string, hostType string) error {
	if ok := Agent.isPluginLoaded(seedMethod); !ok {
		return log.Errorf("Unable to start prepare process, plugin %s not loaded", seedMethod)
	}
	err := Agent.BackupPlugins[seedMethod].Prepare(Agent.Params, hostType)
	return log.Errore(err)
}

func (Agent agent) Restore(seedID string, seedMethod string) error {
	if ok := Agent.isPluginLoaded(seedMethod); !ok {
		return log.Errorf("Unable to start restore process, plugin %s not loaded", seedMethod)
	}
	return log.Errore(Agent.BackupPlugins[seedMethod].Restore(Agent.Params))
}

func (Agent agent) isPluginLoaded(seedMethod string) bool {
	if _, ok := Agent.BackupPlugins[seedMethod]; ok {
		return true
	}
	return false
}

func (Agent agent) Backup(seedID string, seedMethod string, targetHost string, databases []string) error {
	if ok := Agent.isPluginLoaded(seedMethod); !ok {
		return log.Errorf("Unable to start backup process, plugin %s not loaded", seedMethod)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 100)
	wg.Add(1)
	data := Agent.BackupPlugins[seedMethod].Backup(Agent.Params, databases, errs)
	go Agent.sendSeedData(targetHost, data, &wg, errs)
	wg.Wait()
	select {
	case err := <-errs:
		return log.Errore(err)
	default:
		return nil
	}
}

func (Agent agent) sendSeedData(targetHost string, data io.Reader, wg *sync.WaitGroup, errs chan error) {
	var stderr bytes.Buffer
	socatConOpts := fmt.Sprintf("TCP:%s:%s", targetHost, strconv.Itoa(config.Config.SeedPort))
	if config.Config.UseSSL {
		socatConOpts = fmt.Sprintf("openssl-connect:%s:%s,cert=%s", targetHost, strconv.Itoa(config.Config.SeedPort), config.Config.SSLCertFile)
		if len(config.Config.SSLCAFile) != 0 {
			socatConOpts += fmt.Sprintf(",cafile=%s", config.Config.SSLCAFile)
		}
		if config.Config.SSLSkipVerify {
			socatConOpts += ",verify=0"
		}
	}
	cmd := exec.Command("socat", "-u", "EXEC:\"zstd -\"", socatConOpts)
	cmd.Stdin = data
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errs <- fmt.Errorf("Unable to start seed data transfer: %+v", err)
		wg.Done()
	} else {
		defer wg.Done()
	}
}

func (Agent agent) Receive(seedID string, seedMethod string) error {
	if ok := Agent.isPluginLoaded(seedMethod); !ok {
		return log.Errorf("Unable to start recieve process, plugin %s not loaded", seedMethod)
	}
	var stderr bytes.Buffer
	socatConOpts := fmt.Sprintf("TCP-LISTEN:%s,reuseaddr", strconv.Itoa(config.Config.SeedPort))
	if config.Config.UseSSL {
		socatConOpts = fmt.Sprintf("openssl-listen:%s,reuseaddr,cert=%s", strconv.Itoa(config.Config.SeedPort), config.Config.SSLCertFile)
		if len(config.Config.SSLCAFile) != 0 {
			socatConOpts += fmt.Sprintf(",cafile=%s", config.Config.SSLCAFile)
		}
		if config.Config.SSLSkipVerify {
			socatConOpts += ",verify=0"
		}
	}
	cmd := exec.Command("socat", "-u", socatConOpts, "EXEC:\"unzstd - -d\"")
	cmd.Stderr = &stderr
	data, err := cmd.StdoutPipe()
	if err != nil {
		return log.Errorf("Unable to create pipe to recieve backup data: %+v", err)
	}
	err = cmd.Start()
	if err != nil {
		return log.Errorf("Unable to start recieve process: %+v", err)
	}
	err = Agent.BackupPlugins[seedMethod].Receive(Agent.Params, data)
	if err != nil {
		return log.Errorf("Unable to recieve backup data: %+v", err)
	}
	err = cmd.Wait()
	if err != nil {
		return log.Errorf("Unable to recieve backup data: %+v, %s", err, stderr.String())
	}
	return nil
}

func (Agent agent) submitAgent() error {
	url := fmt.Sprintf("%s/api/submit-agent/%s/%d/%s", config.Config.AgentsServer+":"+strconv.Itoa(config.Config.AgentsServerPort), Agent.Hostname, Agent.Port, Agent.Token)
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
func (Agent agent) pingServer() error {
	url := fmt.Sprintf("%s/api/agent-ping", config.Config.AgentsServer+":"+strconv.Itoa(config.Config.AgentsServerPort))
	response, err := httpGet(url)
	if err != nil {
		return log.Errore(err)
	}
	defer response.Body.Close()
	return nil
}

func initilizePlugins(pluginDir string) map[string]BackupPlugin {
	plugins := make(map[string]BackupPlugin)
	pluginFiles, err := filepath.Glob(filepath.Join(pluginDir, "*.so"))
	if err != nil {
		log.Fatalf("Unable to get plugins from %s", pluginDir)
	}
	if pluginFiles == nil {
		log.Errorf("No plugins found in %s", pluginDir)
		return plugins
	}
	for _, file := range pluginFiles {
		var newBackupPlugin BackupPlugin
		pluginName := strings.TrimSuffix(filepath.Base(file), filepath.Ext(filepath.Base(file)))
		plug, err := plugin.Open(file)
		if err != nil {
			log.Errorf("Unable to load plugin %s from %s: %s", filepath.Base(file), pluginDir, err)
			continue
		}
		loadedPlugin, err := plug.Lookup("BackupPlugin")
		if err != nil {
			log.Errorf("Error loading plugin %s from %s: %+v", filepath.Base(file), pluginDir, err)
			continue
		}
		newBackupPlugin, ok := loadedPlugin.(BackupPlugin)
		if !ok {
			log.Errorf("Error loading plugin %s from %s", pluginName, filepath.Join(pluginDir, filepath.Base(file)))
			continue
		}
		plugins[pluginName] = newBackupPlugin
		log.Infof("Succesfully loaded %s plugin", pluginName)
	}
	return plugins
}

// GetAgentInfo returns information about agent to orchestrator
func (Agent agent) GetAgentInfo() AgentInfo {
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
	agentInfo.LogicalVolumes, err = logicalVolumes("", config.Config.Plugins.LVM.SnapshotName)
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
	agentInfo.BackupDir = config.Config.BackupDir
	agentInfo.BackupDirDiskFree, err = getDiskStatistics(config.Config.BackupDir, "free")
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
	availiableSeedMethodsBackup := make(map[string]*SeedMethod)
	availiableSeedMethodsRestore := make(map[string]*SeedMethod)
	for plugin := range Agent.BackupPlugins {
		if Agent.BackupPlugins[plugin].IsAvailiableBackup() {
			availiableSeedMethodsBackup[plugin] = &SeedMethod{
				SupportedEngines:         Agent.BackupPlugins[plugin].SupportedEngines(),
				SupportDatabaseSelection: Agent.BackupPlugins[plugin].SupportDatabaseSelection(),
			}
		}
		if Agent.BackupPlugins[plugin].IsAvailiableRestore() {
			availiableSeedMethodsRestore[plugin] = &SeedMethod{
				SupportedEngines:         Agent.BackupPlugins[plugin].SupportedEngines(),
				SupportDatabaseSelection: Agent.BackupPlugins[plugin].SupportDatabaseSelection(),
			}
		}
	}
	agentInfo.AvailiableSeedMethodsBackup = availiableSeedMethodsBackup
	agentInfo.AvailiableSeedMethodsRestore = availiableSeedMethodsRestore
	return agentInfo
}
