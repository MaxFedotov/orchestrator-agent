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
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/pprof"
	"path"
	"strconv"
	"time"

	"github.com/github/orchestrator-agent/go/config"
	"github.com/github/orchestrator-agent/go/helper/token"
	"github.com/github/orchestrator-agent/go/osagent"
	"github.com/github/orchestrator-agent/go/seed"
	"github.com/go-martini/martini"
	"github.com/martini-contrib/render"
)

type HttpAPI struct{}

var API = HttpAPI{}

// APIResponseCode is an OK/ERROR response code
type APIResponseCode int

const (
	ERROR APIResponseCode = iota
	OK
)

func (this *APIResponseCode) MarshalJSON() ([]byte, error) {
	return json.Marshal(this.String())
}

func (this *APIResponseCode) String() string {
	switch *this {
	case ERROR:
		return "ERROR"
	case OK:
		return "OK"
	}
	return "unknown"
}

// APIResponse is a response returned as JSON to various requests.
type APIResponse struct {
	Code    APIResponseCode
	Message string
	Details interface{}
}

// validateToken validates the request contains a valid token
func (this *HttpAPI) validateToken(r render.Render, req *http.Request) error {
	var requestToken string
	if config.Config.TokenHttpHeader != "" {
		requestToken = req.Header.Get(config.Config.TokenHttpHeader)
	}
	if requestToken == "" {
		requestToken = req.URL.Query().Get("token")
	}
	if requestToken == token.ProcessToken.Hash {
		return nil
	}
	err := errors.New("Invalid token")
	r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
	return err
}

// MountLV mounts a logical volume on config mount point
func (this *HttpAPI) MountLV(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	lv := params["lv"]
	if lv == "" {
		lv = req.URL.Query().Get("lv")
	}
	output, err := osagent.MountLV(config.Config.SnapshotMountPoint, lv, agent.Config.Common.ExecWithSudo)
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, output)
}

// RemoveLV removes a logical volume
func (this *HttpAPI) RemoveLV(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	lv := params["lv"]
	if lv == "" {
		lv = req.URL.Query().Get("lv")
	}
	err := osagent.RemoveLV(lv, agent.Config.Common.ExecWithSudo)
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, err == nil)
}

// Unmount umounts the config mount point
func (this *HttpAPI) Unmount(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	output, err := osagent.Unmount(agent.Config.LVM.SnapshotMountPoint, agent.Config.Common.ExecWithSudo)
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, output)
}

// CreateSnapshot lists dc-local available snapshots for this host
func (this *HttpAPI) CreateSnapshot(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	err := osagent.CreateSnapshot(agent.Config.LVM.CreateSnapshotCommand)
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, err == nil)
}

// MySQLStop shuts down the MySQL service
func (this *HttpAPI) MySQLStop(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	err := osagent.MySQLStop(agent.Config.Common.ExecWithSudo)
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, err == nil)
}

// MySQLStop starts the MySQL service
func (this *HttpAPI) MySQLStart(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	err := osagent.MySQLStart(agent.Config.Common.ExecWithSudo)
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, err == nil)
}

// PostCopy
func (this *HttpAPI) PostCopy(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	err := osagent.PostCopy(agent.Config.Common.PostSeedCommand, agent.Config.Common.ExecWithSudo)
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, err == nil)
}

// ReceiveMySQLSeedData
func (this *HttpAPI) ReceiveMySQLSeedData(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	var err error
	if err = this.validateToken(r, req); err != nil {
		return
	}
	go osagent.ReceiveMySQLSeedData(params["seedId"], agent.Config.Common.ExecWithSudo)
	r.JSON(200, err == nil)
}

// SeedCommandCompleted
func (this *HttpAPI) SeedCommandCompleted(params martini.Params, r render.Render, req *http.Request) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	output := osagent.SeedCommandCompleted(params["seedId"])
	r.JSON(200, output)
}

// SeedCommandCompleted
func (this *HttpAPI) SeedCommandSucceeded(params martini.Params, r render.Render, req *http.Request) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	output := osagent.SeedCommandSucceeded(params["seedId"])
	r.JSON(200, output)
}

// A simple status endpoint to ping to see if the agent is up and responding.  There's not much
// to do here except respond with 200 and OK
// This is pointed to by a configurable endpoint and has a configurable status message
func (this *HttpAPI) Status(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if uint(time.Since(agent.LastTalkback).Seconds()) > config.Config.StatusBadSeconds {
		r.JSON(500, "BAD")
	} else {
		r.JSON(200, "OK")
	}
}

// RelayLogIndexFile returns mysql relay log index file, full path
func (this *HttpAPI) RelayLogIndexFile(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req); err != nil {
		return
	}

	output, err := osagent.GetRelayLogIndexFileName(agent.Info.MySQLDatadir, agent.Config.Common.ExecWithSudo)
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to find relay log index file")
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, output)
}

// RelayLogFiles returns the list of active relay logs
func (this *HttpAPI) RelayLogFiles(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req); err != nil {
		return
	}

	output, err := osagent.GetRelayLogFileNames(agent.Info.MySQLDatadir, agent.Config.Common.ExecWithSudo)
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to find active relay logs")
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, output)
}

// RelayLogFiles returns the list of active relay logs
func (this *HttpAPI) RelayLogEndCoordinates(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req); err != nil {
		return
	}

	coordinates, err := osagent.GetRelayLogEndCoordinates(agent.Info.MySQLDatadir, agent.Config.Common.ExecWithSudo)
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to get relay log end coordinates")
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, coordinates)
}

// RelaylogContentsTail returns contents of relay logs, from given position to the very last entry
func (this *HttpAPI) RelaylogContentsTail(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req); err != nil {
		return
	}

	var err error
	var startPosition int64
	if startPosition, err = strconv.ParseInt(params["start"], 10, 0); err != nil {
		err = fmt.Errorf("Cannot parse startPosition: %s", err.Error())
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	firstRelaylog := params["relaylog"]
	var parseRelaylogs []string
	if existingRelaylogs, err := osagent.GetRelayLogFileNames(agent.Info.MySQLDatadir, agent.Config.Common.ExecWithSudo); err != nil {
		agent.Logger.WithField("error", err).Error("Unable to find active relay logs")
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	} else {
		for i, relaylog := range existingRelaylogs {
			if (firstRelaylog == relaylog) || (firstRelaylog == path.Base(relaylog)) {
				// found the relay log we want to start with
				parseRelaylogs = existingRelaylogs[i:]
			}
		}
	}

	output, err := osagent.MySQLBinlogBinaryContents(parseRelaylogs, startPosition, 0, agent.Config.Common.ExecWithSudo)
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, output)
}

// binlogContents returns contents of binary log entries
func (this *HttpAPI) binlogContents(params martini.Params, r render.Render, req *http.Request, agent *Agent,
	contentsFunc func(binlogFiles []string, startPosition int64, stopPosition int64, ExecWithSudo bool) (string, error),
) {
	if err := this.validateToken(r, req); err != nil {
		return
	}

	var err error
	var startPosition, stopPosition int64
	if start := req.URL.Query().Get("start"); start != "" {
		if startPosition, err = strconv.ParseInt(start, 10, 0); err != nil {
			r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
			return
		}
	}
	if stop := req.URL.Query().Get("stop"); stop != "" {
		if stopPosition, err = strconv.ParseInt(stop, 10, 0); err != nil {
			r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
			return
		}
	}
	binlogFileNames := req.URL.Query()["binlog"]
	output, err := osagent.MySQLBinlogContents(binlogFileNames, startPosition, stopPosition, agent.Config.Common.ExecWithSudo)
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, output)
}

// BinlogContents returns contents of binary log entries
func (this *HttpAPI) BinlogContents(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	this.binlogContents(params, r, req, agent, osagent.MySQLBinlogContents)
}

// BinlogBinaryContents returns contents of binary log entries
func (this *HttpAPI) BinlogBinaryContents(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	this.binlogContents(params, r, req, agent, osagent.MySQLBinlogBinaryContents)
}

// ApplyRelaylogContents reads binlog contents from request's body and applies them locally
func (this *HttpAPI) ApplyRelaylogContents(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	defer req.Body.Close()

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	err = osagent.ApplyRelaylogContents(body, agent.Config.Common.ExecWithSudo, agent.Config.Mysql.SeedUser, agent.Config.Mysql.SeedPassword)
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to apply relay log contents")
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, "OK")
}

// getAgent returns information about agent
func (this *HttpAPI) getAgent(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	output := agent.GetAgentInfo()
	r.JSON(200, output)
}

// Prepare starts prepare stage for seed
func (this *HttpAPI) Prepare(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	var seedMethod seed.Method
	var seedSide seed.Side
	var ok bool
	seedID, err := strconv.Atoi(params["seedID"])
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to parse seedID")
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	if seedMethod, ok = seed.ToMethod[params["seedMethod"]]; !ok {
		agent.Logger.WithField("seedMethod", params["seedMethod"]).Error("Seed method undefinded")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Seed method undefinded"})
		return
	}
	if _, ok = agent.Params.AvailiableSeedMethods[seedMethod]; !ok {
		agent.Logger.WithField("seedMethod", params["seedMethod"]).Error("Seed method unavailiable on agent")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Seed method unavailiable on agent"})
		return
	}
	if seedSide, ok = seed.ToSide[params["seedSide"]]; !ok {
		agent.Logger.WithField("seedSide", params["seedSide"]).Error("Seed side undefinded")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Seed side undefinded"})
		return
	}
	agent.Lock()
	defer agent.Unlock()
	agent.ActiveSeedID = seedID
	go agent.SeedMethods[seedMethod].Prepare(seedSide)
	r.Text(202, "Started")
}

// Backup starts Backup stage for seed
func (this *HttpAPI) Backup(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	var seedMethod seed.Method
	var ok bool
	seedID, err := strconv.Atoi(params["seedID"])
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to parse seedID")
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	if seedID != agent.ActiveSeedID {
		agent.Logger.WithField("seedID", seedID).Error("Unable to start backup. SeedID not found")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Unable to start backup. SeedID not found"})
		return
	}
	mysqlPort, err := strconv.Atoi(params["mysqlPort"])
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to parse MySQL port")
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	if seedMethod, ok = seed.ToMethod[params["seedMethod"]]; !ok {
		agent.Logger.WithField("seedMethod", params["seedMethod"]).Error("Seed method undefinded")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Seed method undefinded"})
		return
	}
	if _, ok = agent.Params.AvailiableSeedMethods[seedMethod]; !ok {
		agent.Logger.WithField("seedMethod", params["seedMethod"]).Error("Seed method unavailiable on agent")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Seed method unavailiable on agent"})
		return
	}
	seedHost := params["seedHost"]
	go agent.SeedMethods[seedMethod].Backup(seedHost, mysqlPort)
	r.Text(202, "Started")
}

// Restore starts Restore stage for seed
func (this *HttpAPI) Restore(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	var seedMethod seed.Method
	var ok bool
	seedID, err := strconv.Atoi(params["seedID"])
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to parse seedID")
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	if seedID != agent.ActiveSeedID {
		agent.Logger.WithField("seedID", seedID).Error("Unable to start restore. SeedID not found")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Unable to start restore. SeedID not found"})
		return
	}
	if seedMethod, ok = seed.ToMethod[params["seedMethod"]]; !ok {
		agent.Logger.WithField("seedMethod", params["seedMethod"]).Error("Seed method undefinded")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Seed method undefinded"})
		return
	}
	if _, ok = agent.Params.AvailiableSeedMethods[seedMethod]; !ok {
		agent.Logger.WithField("seedMethod", params["seedMethod"]).Error("Seed method unavailiable on agent")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Seed method unavailiable on agent"})
		return
	}
	go agent.SeedMethods[seedMethod].Restore()
	r.Text(202, "Started")
}

// Cleanup starts Cleanup stage for seed
func (this *HttpAPI) Cleanup(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	var seedMethod seed.Method
	var seedSide seed.Side
	var ok bool
	seedID, err := strconv.Atoi(params["seedID"])
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to parse seedID")
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	if seedID != agent.ActiveSeedID {
		agent.Logger.WithField("seedID", seedID).Error("Unable to start cleanup. SeedID not found")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Unable to start cleanup. SeedID not found"})
		return
	}
	if seedMethod, ok = seed.ToMethod[params["seedMethod"]]; !ok {
		agent.Logger.WithField("seedMethod", params["seedMethod"]).Error("Seed method undefinded")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Seed method undefinded"})
		return
	}
	if _, ok = agent.Params.AvailiableSeedMethods[seedMethod]; !ok {
		agent.Logger.WithField("seedMethod", params["seedMethod"]).Error("Seed method unavailiable on agent")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Seed method unavailiable on agent"})
		return
	}
	if seedSide, ok = seed.ToSide[params["seedSide"]]; !ok {
		agent.Logger.WithField("seedSide", params["seedSide"]).Error("Seed side undefinded")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Seed side undefinded"})
		return
	}
	go agent.SeedMethods[seedMethod].Cleanup(seedSide)
	r.Text(202, "Started")
}

// GetMetadata returns metadata (gtidExecute or positional) for seed
func (this *HttpAPI) GetMetadata(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	var seedMethod seed.Method
	var ok bool
	seedID, err := strconv.Atoi(params["seedID"])
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to parse seedID")
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	if seedID != agent.ActiveSeedID {
		agent.Logger.WithField("seedID", seedID).Error("Unable to start cleanup. SeedID not found")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Unable to start cleanup. SeedID not found"})
		return
	}
	if seedMethod, ok = seed.ToMethod[params["seedMethod"]]; !ok {
		agent.Logger.WithField("seedMethod", params["seedMethod"]).Error("Seed method undefinded")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Seed method undefinded"})
		return
	}
	if _, ok = agent.Params.AvailiableSeedMethods[seedMethod]; !ok {
		agent.Logger.WithField("seedMethod", params["seedMethod"]).Error("Seed method unavailiable on agent")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Seed method unavailiable on agent"})
		return
	}
	metadata, err := agent.SeedMethods[seedMethod].GetMetadata()
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to get backup metadata")
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, metadata)
}

// AbortSeed tries to seed process
func (this *HttpAPI) AbortSeed(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	seedID, err := strconv.Atoi(params["seedID"])
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to parse seedID")
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	if seedID != agent.ActiveSeedID {
		agent.Logger.WithField("seedID", seedID).Error("Unable to abort seed. SeedID not found")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Unable to abort seedp. SeedID not found"})
		return
	}
	agent.Lock()
	defer agent.Unlock()
	if agent.SeedStageStatus[seedID].Status != seed.Running || agent.SeedStageStatus[seedID].Command == nil {
		agent.Logger.WithField("seedID", seedID).Error("Unable to abort seed. Seed not running")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Unable to abort seedp. Seed not running"})
		return
	}
	agent.SeedStageStatus[seedID].Command.Kill()
	r.Text(200, "killed")
}

func (this *HttpAPI) SeedStatus(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	seedID, err := strconv.Atoi(params["seedID"])
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to parse seedID")
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	if _, ok := agent.SeedStageStatus[seedID]; !ok {
		r.JSON(500, &APIResponse{Code: ERROR, Message: "SeedID not found"})
		return
	}
	r.JSON(200, agent.SeedStageStatus[seedID])
}

// TODELETE
func (this *HttpAPI) getAgentParams(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	r.JSON(200, agent.Params)
}

/*
func (this *HttpAPI) RunCommand(params martini.Params, r render.Render, req *http.Request) {
	if err := this.validateToken(r, req); err != nil {
		return
	}

	if _, ok := config.Config.CustomCommands[params["cmd"]]; ok {
		commandOutput, err := osagent.ExecCustomCmdWithOutput(params["cmd"])
		if err != nil {
			r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
			return
		}

		r.JSON(200, &APIResponse{Code: OK, Message: string(commandOutput)})
		return
	} else {
		err := fmt.Errorf("%s : Command not found", params["cmd"])
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
}

// DiskUsage returns the number of bytes of a give ndirectory (recursive)
func (this *HttpAPI) DiskUsage(params martini.Params, r render.Render, req *http.Request) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	path := req.URL.Query().Get("path")

	output, err := osagent.DiskUsage(path)
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, output)
}


// DeleteMySQLDataDir compeltely erases MySQL data directory. Use with care!
func (this *HttpAPI) DeleteMySQLDataDir(params martini.Params, r render.Render, req *http.Request) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	err := osagent.DeleteMySQLDataDir()
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, err == nil)
}

// ListSnapshotsLogicalVolumes lists logical volumes by pattern
func (this *HttpAPI) ListSnapshotsLogicalVolumes(params martini.Params, r render.Render, req *http.Request) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	output, err := osagent.LogicalVolumes("", config.Config.SnapshotVolumesFilter)
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, output)
}

// LogicalVolume lists a logical volume by name/path/mount point
func (this *HttpAPI) LogicalVolume(params martini.Params, r render.Render, req *http.Request) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	lv := params["lv"]
	if lv == "" {
		lv = req.URL.Query().Get("lv")
	}
	output, err := osagent.LogicalVolumes(lv, "")
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, output)
}

// GetMount shows the configured mount point's status
func (this *HttpAPI) GetMount(params martini.Params, r render.Render, req *http.Request) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	output, err := osagent.GetMount(config.Config.SnapshotMountPoint)
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, output)
}


// LocalSnapshots lists dc-local available snapshots for this host
func (this *HttpAPI) AvailableLocalSnapshots(params martini.Params, r render.Render, req *http.Request) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	output, err := osagent.AvailableSnapshots(true)
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, output)
}

// Snapshots lists available snapshots for this host
func (this *HttpAPI) AvailableSnapshots(params martini.Params, r render.Render, req *http.Request) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	output, err := osagent.AvailableSnapshots(false)
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, output)
}

// returns rows in tail of mysql error log
func (this *HttpAPI) MySQLErrorLogTail(params martini.Params, r render.Render, req *http.Request) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	output, err := osagent.MySQLErrorLogTail()
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, output)
}

// MySQLRunning checks whether the MySQL service is up
func (this *HttpAPI) MySQLRunning(params martini.Params, r render.Render, req *http.Request) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	output, err := osagent.MySQLRunning()
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, output)
}


// SendMySQLSeedData
func (this *HttpAPI) SendMySQLSeedData(params martini.Params, r render.Render, req *http.Request) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	mount, err := osagent.GetMount(config.Config.SnapshotMountPoint)
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	go osagent.SendMySQLSeedData(params["targetHost"], mount.MySQLDataPath, params["seedId"])
	r.JSON(200, err == nil)
}

// ListLogicalVolumes lists logical volumes by pattern
func (this *HttpAPI) ListLogicalVolumes(params martini.Params, r render.Render, req *http.Request) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	output, err := osagent.LogicalVolumes("", params["pattern"])
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, output)
}

// Hostname provides information on this process
func (this *HttpAPI) Hostname(params martini.Params, r render.Render) {
	hostname, err := os.Hostname()
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, hostname)
}

// MySQLDiskUsage returns the number of bytes on the MySQL datadir
func (this *HttpAPI) MySQLDiskUsage(params martini.Params, r render.Render, req *http.Request) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	datadir, err := osagent.GetMySQLDataDir()

	output, err := osagent.DiskUsage(datadir)
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, output)
}

// MySQLPort returns the (heuristic) port on which MySQL executes
func (this *HttpAPI) MySQLPort(params martini.Params, r render.Render, req *http.Request) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	output, err := osagent.GetMySQLPort()
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, output)
}

// GetMySQLDataDirAvailableDiskSpace returns the number of bytes free within the MySQL datadir mount
func (this *HttpAPI) GetMySQLDataDirAvailableDiskSpace(params martini.Params, r render.Render, req *http.Request) {
	if err := this.validateToken(r, req); err != nil {
		return
	}
	output, err := osagent.GetMySQLDataDirAvailableDiskSpace()
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, output)
}

*/

// RegisterRequests makes for the de-facto list of known API calls
func (this *HttpAPI) RegisterRequests(m *martini.ClassicMartini) {
	// commands LVM
	m.Get("/api/mountlv", this.MountLV)                // MountLV mounts a snapshot on config mount point (Backup) ++
	m.Get("/api/removelv", this.RemoveLV)              // RemoveLV removes a snapshot ++
	m.Get("/api/umount", this.Unmount)                 // Unmount umounts the config mount point (Cleanup) ++
	m.Get("/api/create-snapshot", this.CreateSnapshot) // CreateSnapshot creates snapshot for this host ++
	m.Get(config.Config.StatusEndpoint, this.Status)

	// commands MySQL
	m.Get("/api/mysql-stop", this.MySQLStop)
	m.Get("/api/mysql-start", this.MySQLStart)

	// status
	m.Get("/api/get-agent", this.getAgent)
	m.Get("/api/get-agent-params", this.getAgentParams) // TO DELETE
	//m.Get("/api/seed-status", this.seedStatus)

	// seed process
	m.Get("/api/prepare/:seedID/:seedMethod/:seedSide", this.Prepare)
	m.Get("/api/backup/:seedID/:seedMethod/:seedHost/:mysqlPort", this.Backup)
	m.Get("/api/restore/:seedID/:seedMethod", this.Restore)
	m.Get("/api/cleanup/:seedID/:seedMethod/:seedSide", this.Cleanup)
	m.Get("/api/get-metadata/:seedID/:seedMethod", this.GetMetadata)
	m.Get("/api/abort-seed/:seedID", this.AbortSeed)
	m.Get("/api/seed-status/:seedID", this.SeedStatus)

	// ??
	m.Get("/api/post-copy", this.PostCopy)
	m.Get("/api/receive-mysql-seed-data/:seedId", this.ReceiveMySQLSeedData)
	//m.Get("/api/send-mysql-seed-data/:targetHost/:seedId", this.SendMySQLSeedData)
	m.Get("/api/seed-command-completed/:seedId", this.SeedCommandCompleted)
	m.Get("/api/seed-command-succeeded/:seedId", this.SeedCommandSucceeded)

	/* TO DELETE
	m.Get("/api/delete-mysql-datadir", this.DeleteMySQLDataDir)
	m.Get("/api/hostname", this.Hostname)
	m.Get("/api/mysql-du", this.MySQLDiskUsage)
	m.Get("/api/mysql-port", this.MySQLPort)
	m.Get("/api/mysql-datadir-available-space", this.GetMySQLDataDirAvailableDiskSpace)
	m.Get("/api/mysql-error-log-tail", this.MySQLErrorLogTail)
	m.Get("/api/mysql-status", this.MySQLRunning)
	m.Get("/api/available-snapshots-local", this.AvailableLocalSnapshots) // LocalSnapshots lists dc-local available snapshots for this host
	m.Get("/api/available-snapshots", this.AvailableSnapshots)            // Snapshots lists available snapshots for this host
	m.Get("/api/lvs-snapshots", this.ListSnapshotsLogicalVolumes) // ListSnapshotsLogicalVolumes lists logical volumes by pattern
	m.Get("/api/mount", this.GetMount)                            // GetMount shows the configured mount point's status
	m.Get("/api/du", this.DiskUsage)
	m.Get("/api/lv", this.LogicalVolume)
	m.Get("/api/lv/:lv", this.LogicalVolume)
	m.Get("/api/lvs", this.ListLogicalVolumes)
	m.Get("/api/lvs/:pattern", this.ListLogicalVolumes)
	*/

	// unused
	m.Get("/api/mysql-binlog-binary-contents", this.BinlogBinaryContents)
	m.Get("/api/mysql-relay-log-index-file", this.RelayLogIndexFile)
	m.Get("/api/mysql-relay-log-files", this.RelayLogFiles)
	m.Get("/api/mysql-relay-log-end-coordinates", this.RelayLogEndCoordinates)
	m.Get("/api/mysql-binlog-contents", this.BinlogContents)

	// called by orchestrator but never used
	m.Get("/api/mysql-relaylog-contents-tail/:relaylog/:start", this.RelaylogContentsTail)
	m.Post("/api/apply-relaylog-contents", this.ApplyRelaylogContents)

	// to delete
	// m.Get("/api/custom-commands/:cmd", this.RunCommand)

	// list all the /debug/ endpoints we want
	m.Get("/debug/pprof", pprof.Index)
	m.Get("/debug/pprof/cmdline", pprof.Cmdline)
	m.Get("/debug/pprof/profile", pprof.Profile)
	m.Get("/debug/pprof/symbol", pprof.Symbol)
	m.Post("/debug/pprof/symbol", pprof.Symbol)
	m.Get("/debug/pprof/block", pprof.Handler("block").ServeHTTP)
	m.Get("/debug/pprof/heap", pprof.Handler("heap").ServeHTTP)
	m.Get("/debug/pprof/goroutine", pprof.Handler("goroutine").ServeHTTP)
	m.Get("/debug/pprof/threadcreate", pprof.Handler("threadcreate").ServeHTTP)
}
