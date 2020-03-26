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

	"github.com/github/orchestrator-agent/go/helper/cmd"
	"github.com/github/orchestrator-agent/go/osagent"
	"github.com/github/orchestrator-agent/go/seed"
	"github.com/github/orchestrator/go/config"
	"github.com/go-martini/martini"
	"github.com/martini-contrib/render"
	log "github.com/sirupsen/logrus"
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
func (this *HttpAPI) validateToken(r render.Render, req *http.Request, agent *Agent) error {
	var requestToken string

	if agent.Config.Common.TokenHTTPHeader != "" {
		requestToken = req.Header.Get(agent.Config.Common.TokenHTTPHeader)
	}
	if requestToken == "" {
		requestToken = req.URL.Query().Get("token")
	}
	if requestToken == agent.Info.Token {
		return nil
	}
	err := errors.New("Invalid token")
	r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
	return err
}

// MountLV mounts a logical volume on config mount point
func (this *HttpAPI) MountLV(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req, agent); err != nil {
		return
	}
	lv := params["lv"]
	if lv == "" {
		lv = req.URL.Query().Get("lv")
	}
	output, err := osagent.MountLV(agent.Config.Common.BackupDir, lv, agent.Config.Common.ExecWithSudo)
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, output)
}

// RemoveLV removes a logical volume
func (this *HttpAPI) RemoveLV(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req, agent); err != nil {
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
	if err := this.validateToken(r, req, agent); err != nil {
		return
	}
	err := osagent.Unmount(agent.Config.Common.BackupDir, agent.Config.Common.ExecWithSudo)
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, err == nil)
}

// CreateSnapshot lists dc-local available snapshots for this host
func (this *HttpAPI) CreateSnapshot(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req, agent); err != nil {
		return
	}
	err := osagent.CreateSnapshot(agent.Config.LVM.CreateSnapshotCommand, agent.Config.Common.ExecWithSudo)
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, err == nil)
}

// MySQLStop shuts down the MySQL service
func (this *HttpAPI) MySQLStop(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req, agent); err != nil {
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
	if err := this.validateToken(r, req, agent); err != nil {
		return
	}
	err := osagent.MySQLStart(agent.Config.Common.ExecWithSudo)
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, err == nil)
}

// MySQLStop starts the MySQL service
func (this *HttpAPI) MySQLErrorLogTail(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req, agent); err != nil {
		return
	}
	output, err := agent.GetMySQLErrorLog()
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, output)
}

// A simple status endpoint to ping to see if the agent is up and responding.  There's not much
// to do here except respond with 200 and OK
// This is pointed to by a configurable endpoint and has a configurable status message
func (this *HttpAPI) Status(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if time.Since(agent.LastTalkback).Seconds() > agent.Config.Common.StatusBadSeconds.Seconds() {
		r.JSON(500, "BAD")
	} else {
		r.JSON(200, "OK")
	}
}

// RelayLogIndexFile returns mysql relay log index file, full path
func (this *HttpAPI) RelayLogIndexFile(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req, agent); err != nil {
		return
	}

	output, err := osagent.GetRelayLogIndexFileName(agent.MySQLDatadir, agent.Config.Common.ExecWithSudo)
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to find relay log index file")
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, output)
}

// RelayLogFiles returns the list of active relay logs
func (this *HttpAPI) RelayLogFiles(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req, agent); err != nil {
		return
	}

	output, err := osagent.GetRelayLogFileNames(agent.MySQLDatadir, agent.Config.Common.ExecWithSudo)
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to find active relay logs")
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, output)
}

// RelayLogFiles returns the list of active relay logs
func (this *HttpAPI) RelayLogEndCoordinates(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req, agent); err != nil {
		return
	}

	coordinates, err := osagent.GetRelayLogEndCoordinates(agent.MySQLDatadir, agent.Config.Common.ExecWithSudo)
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to get relay log end coordinates")
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, coordinates)
}

// RelaylogContentsTail returns contents of relay logs, from given position to the very last entry
func (this *HttpAPI) RelaylogContentsTail(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req, agent); err != nil {
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
	if existingRelaylogs, err := osagent.GetRelayLogFileNames(agent.MySQLDatadir, agent.Config.Common.ExecWithSudo); err != nil {
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
	if err := this.validateToken(r, req, agent); err != nil {
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
	if err := this.validateToken(r, req, agent); err != nil {
		return
	}
	defer req.Body.Close()

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	err = osagent.ApplyRelaylogContents(body, agent.Config.Common.ExecWithSudo, agent.Config.Mysql.User, agent.Config.Mysql.Password)
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to apply relay log contents")
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	r.JSON(200, "OK")
}

// getAgent returns information about agent
func (this *HttpAPI) getAgentData(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req, agent); err != nil {
		return
	}
	output := agent.GetAgentData()
	r.JSON(200, output)
}

// Prepare starts prepare stage for seed
func (this *HttpAPI) Prepare(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req, agent); err != nil {
		return
	}
	var seedMethod seed.Method
	var seedSide seed.Side
	var ok bool
	seedStage := seed.Prepare
	seedID, err := strconv.ParseInt(params["seedID"], 10, 64)
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
	if _, ok = agent.AvailiableSeedMethods[seedMethod]; !ok {
		agent.Logger.WithField("seedMethod", params["seedMethod"]).Error("Seed method unavailiable on agent")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Seed method unavailiable on agent"})
		return
	}
	if seedSide, ok = seed.ToSide[params["seedSide"]]; !ok {
		agent.Logger.WithField("seedSide", params["seedSide"]).Error("Seed side undefinded")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Seed side undefinded"})
		return
	}
	if agent.ActiveSeed.SeedID == seedID && agent.ActiveSeed.Stage == seedStage && (agent.ActiveSeed.StagesDetails[seedStage].Status == seed.Running || agent.ActiveSeed.StagesDetails[seedStage].Status == seed.Completed) {
		r.Text(202, fmt.Sprintf("%s stage already started for seed", seedStage.String()))
	}
	seedStagesDetails := make(map[seed.Stage]*seed.SeedStageState)
	agent.Lock()
	agent.ActiveSeed.SeedID = seedID
	agent.ActiveSeed.Stage = seedStage
	agent.ActiveSeed.SeedStatus = seed.Running
	agent.ActiveSeed.Side = seedSide
	agent.ActiveSeed.Method = seedMethod
	agent.ActiveSeed.StagesDetails = seedStagesDetails
	agent.Unlock()
	go agent.SeedMethods[seedMethod].Prepare(seedSide)
	r.Text(202, "Started")
}

// Backup starts Backup stage for seed
func (this *HttpAPI) Backup(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req, agent); err != nil {
		return
	}
	var seedMethod seed.Method
	var ok bool
	seedStage := seed.Backup
	seedID, err := strconv.ParseInt(params["seedID"], 10, 64)
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to parse seedID")
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	if seedID != agent.ActiveSeed.SeedID {
		agent.Logger.WithField("seedID", seedID).Error("Unable to start backup stage. SeedID not found")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Unable to start backup stage. SeedID not found"})
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
	if _, ok = agent.AvailiableSeedMethods[seedMethod]; !ok {
		agent.Logger.WithField("seedMethod", params["seedMethod"]).Error("Seed method unavailiable on agent")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Seed method unavailiable on agent"})
		return
	}
	seedHost := params["seedHost"]
	if agent.ActiveSeed.SeedID == seedID && agent.ActiveSeed.Stage == seedStage && (agent.ActiveSeed.StagesDetails[seedStage].Status == seed.Running || agent.ActiveSeed.StagesDetails[seedStage].Status == seed.Completed) {
		r.Text(202, fmt.Sprintf("%s stage already started for seed", seedStage.String()))
	}
	agent.Lock()
	agent.ActiveSeed.Stage = seedStage
	agent.Unlock()
	go agent.SeedMethods[seedMethod].Backup(seedHost, mysqlPort)
	r.Text(202, "Started")
}

// Restore starts Restore stage for seed
func (this *HttpAPI) Restore(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req, agent); err != nil {
		return
	}
	var seedMethod seed.Method
	var ok bool
	seedStage := seed.Restore
	seedID, err := strconv.ParseInt(params["seedID"], 10, 64)
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to parse seedID")
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	if seedID != agent.ActiveSeed.SeedID {
		agent.Logger.WithField("seedID", seedID).Error("Unable to start restore stage. SeedID not found")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Unable to start restore stage. SeedID not found"})
		return
	}
	if seedMethod, ok = seed.ToMethod[params["seedMethod"]]; !ok {
		agent.Logger.WithField("seedMethod", params["seedMethod"]).Error("Seed method undefinded")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Seed method undefinded"})
		return
	}
	if _, ok = agent.AvailiableSeedMethods[seedMethod]; !ok {
		agent.Logger.WithField("seedMethod", params["seedMethod"]).Error("Seed method unavailiable on agent")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Seed method unavailiable on agent"})
		return
	}
	if agent.ActiveSeed.SeedID == seedID && agent.ActiveSeed.Stage == seedStage && (agent.ActiveSeed.StagesDetails[seedStage].Status == seed.Running || agent.ActiveSeed.StagesDetails[seedStage].Status == seed.Completed) {
		r.Text(202, fmt.Sprintf("%s stage already started for seed", seedStage.String()))
	}
	agent.Lock()
	agent.ActiveSeed.Stage = seedStage
	agent.Unlock()
	go agent.SeedMethods[seedMethod].Restore()
	r.Text(202, "Started")
}

// GetMetadata returns metadata (gtidExecute or positional) for seed
func (this *HttpAPI) GetMetadata(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req, agent); err != nil {
		return
	}
	var seedMethod seed.Method
	var ok bool
	seedStage := seed.ConnectSlave
	seedID, err := strconv.ParseInt(params["seedID"], 10, 64)
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to parse seedID")
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	if seedID != agent.ActiveSeed.SeedID {
		agent.Logger.WithField("seedID", seedID).Error("Unable to get metadata for seed. SeedID not found")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Unable to get metadata for seed. SeedID not found"})
		return
	}
	if seedMethod, ok = seed.ToMethod[params["seedMethod"]]; !ok {
		agent.Logger.WithField("seedMethod", params["seedMethod"]).Error("Seed method undefinded")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Seed method undefinded"})
		return
	}
	if _, ok = agent.AvailiableSeedMethods[seedMethod]; !ok {
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
	agent.Lock()
	agent.ActiveSeed.Stage = seedStage
	agent.Unlock()
	r.JSON(200, metadata)
}

// Cleanup starts Cleanup stage for seed
func (this *HttpAPI) Cleanup(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req, agent); err != nil {
		return
	}
	var seedMethod seed.Method
	var seedSide seed.Side
	var ok bool
	seedStage := seed.Cleanup
	seedID, err := strconv.ParseInt(params["seedID"], 10, 64)
	if err != nil {
		agent.Logger.WithField("error", err).Error("Unable to parse seedID")
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	if seedID != agent.ActiveSeed.SeedID {
		agent.Logger.WithField("seedID", seedID).Error("Unable to start cleanup stage. SeedID not found")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Unable to start cleanup stage. SeedID not found"})
		return
	}
	if seedMethod, ok = seed.ToMethod[params["seedMethod"]]; !ok {
		agent.Logger.WithField("seedMethod", params["seedMethod"]).Error("Seed method undefinded")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Seed method undefinded"})
		return
	}
	if _, ok = agent.AvailiableSeedMethods[seedMethod]; !ok {
		agent.Logger.WithField("seedMethod", params["seedMethod"]).Error("Seed method unavailiable on agent")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Seed method unavailiable on agent"})
		return
	}
	if seedSide, ok = seed.ToSide[params["seedSide"]]; !ok {
		agent.Logger.WithField("seedSide", params["seedSide"]).Error("Seed side undefinded")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Seed side undefinded"})
		return
	}
	if agent.ActiveSeed.SeedID == seedID && agent.ActiveSeed.Stage == seedStage && (agent.ActiveSeed.StagesDetails[seedStage].Status == seed.Running || agent.ActiveSeed.StagesDetails[seedStage].Status == seed.Completed) {
		r.Text(202, fmt.Sprintf("%s stage already started for seed", seedStage.String()))
	}
	agent.Lock()
	agent.ActiveSeed.Stage = seedStage
	agent.Unlock()
	go agent.SeedMethods[seedMethod].Cleanup(seedSide)
	r.Text(202, "Started")
}

// postSeedCmd executes custom-commands.post-seed-command from config custom-commands config section after seed will be completed
func (this *HttpAPI) postSeedCmd(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req, agent); err != nil {
		return
	}
	seedID, err := strconv.ParseInt(params["seedID"], 10, 64)
	if err != nil {
		agent.Logger.WithFields(log.Fields{"error": err, "seedID": params["seedID"]}).Error("Unable to parse seedID")
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	if seedID != agent.ActiveSeed.SeedID {
		agent.Logger.WithFields(log.Fields{"seedID": params["seedID"]}).Error("Unable to execute post seed command. SeedID not found")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Unable to execute post seed command. SeedID not found"})
		return
	}
	if _, ok := agent.Config.CustomCommands["post-seed-command"]; ok {
		commandOutput, err := cmd.CommandOutput(agent.Config.CustomCommands["post-seed-command"].Cmd, agent.Config.Common.ExecWithSudo)
		if err != nil {
			r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
			return
		}
		r.JSON(200, &APIResponse{Code: OK, Message: string(commandOutput)})
		return
	}
	r.JSON(200, err == nil)
}

// AbortSeed tries to abort seed process
func (this *HttpAPI) AbortSeedStage(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	var seedStage seed.Stage
	var ok bool
	if err := this.validateToken(r, req, agent); err != nil {
		return
	}
	seedID, err := strconv.ParseInt(params["seedID"], 10, 64)
	if err != nil {
		agent.Logger.WithFields(log.Fields{"error": err, "seedID": params["seedID"], "seedStage": params["seedStage"]}).Error("Unable to parse seedID")
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	if seedID != agent.ActiveSeed.SeedID {
		agent.Logger.WithFields(log.Fields{"seedID": params["seedID"], "seedStage": params["seedStage"]}).Error("Unable to abort seed stage. SeedID not found")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Unable to abort seed stage. SeedID not found"})
		return
	}
	if seedStage, ok = seed.ToSeedStage[params["seedStage"]]; !ok {
		agent.Logger.WithFields(log.Fields{"seedID": params["seedID"], "seedStage": params["seedStage"]}).Error("Seed stage undefinded")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Seed stage undefinded"})
		return
	}
	if seedID != agent.ActiveSeed.SeedID {
		agent.Logger.WithField("seedID", seedID).Error("Unable to abort seed stage. SeedID not found")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Unable to abort seed stage. SeedID not found"})
		return
	}
	agent.Lock()
	defer agent.Unlock()
	if agent.ActiveSeed.StagesDetails[seedStage].Status == seed.Running || agent.ActiveSeed.StagesDetails[seedStage].Command == nil {
		agent.Logger.WithFields(log.Fields{"seedID": params["seedID"], "seedStage": params["seedStage"]}).Error("Unable to abort seed. Seed not running")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Unable to abort seed. Seed not running"})
		return
	}
	agent.ActiveSeed.StagesDetails[seedStage].Command.Kill()
	agent.ActiveSeed.SeedStatus = seed.Cancelled
	r.Text(200, "killed")
}

func (this *HttpAPI) SeedStageState(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	var seedStage seed.Stage
	var ok bool
	if err := this.validateToken(r, req, agent); err != nil {
		return
	}
	seedID, err := strconv.ParseInt(params["seedID"], 10, 64)
	if err != nil {
		agent.Logger.WithFields(log.Fields{"error": err, "seedID": params["seedID"], "seedStage": params["seedStage"]}).Error("Unable to parse seedID")
		r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
		return
	}
	if seedStage, ok = seed.ToSeedStage[params["seedStage"]]; !ok {
		agent.Logger.WithFields(log.Fields{"seedID": params["seedID"], "seedStage": params["seedStage"]}).Error("Seed stage undefinded")
		r.JSON(500, &APIResponse{Code: ERROR, Message: "Seed stage undefinded"})
		return
	}
	if seedID != agent.ActiveSeed.SeedID {
		agent.Logger.WithFields(log.Fields{"seedID": params["seedID"]}).Error("Cannot found seedID")
		r.JSON(500, &APIResponse{Code: ERROR, Message: fmt.Sprintf("SeedID %d not found", seedID)})
		return
	}
	if _, ok := agent.ActiveSeed.StagesDetails[seedStage]; !ok {
		agent.Logger.WithFields(log.Fields{"seedID": params["seedID"], "seedStage": params["seedStage"]}).Error("Cannot found seedStage for seedID")
		r.JSON(500, &APIResponse{Code: ERROR, Message: fmt.Sprintf("SeedID %d on stage %s not found", seedID, seedStage)})
		return
	}
	r.JSON(200, agent.ActiveSeed.StagesDetails[seedStage])
}

func (this *HttpAPI) ActiveSeed(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req, agent); err != nil {
		return
	}
	r.JSON(200, agent.ActiveSeed)
}

func (this *HttpAPI) RunCommand(params martini.Params, r render.Render, req *http.Request, agent *Agent) {
	if err := this.validateToken(r, req, agent); err != nil {
		return
	}
	if _, ok := agent.Config.CustomCommands[params["cmd"]]; ok {
		commandOutput, err := cmd.CommandOutput(agent.Config.CustomCommands[params["cmd"]].Cmd, agent.Config.Common.ExecWithSudo)
		if err != nil {
			r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
			return
		}
		r.JSON(200, &APIResponse{Code: OK, Message: string(commandOutput)})
		return
	}
	err := fmt.Errorf("%s : Command not found", params["cmd"])
	r.JSON(500, &APIResponse{Code: ERROR, Message: err.Error()})
	return
}

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
	m.Get("/api/mysql-error-log-tail", this.MySQLErrorLogTail)

	// status
	m.Get("/api/get-agent-data", this.getAgentData)

	// seed process
	m.Get("/api/prepare/:seedID/:seedMethod/:seedSide", this.Prepare)
	m.Get("/api/backup/:seedID/:seedMethod/:seedHost/:mysqlPort", this.Backup)
	m.Get("/api/restore/:seedID/:seedMethod", this.Restore)
	m.Get("/api/cleanup/:seedID/:seedMethod/:seedSide", this.Cleanup)
	m.Get("/api/get-metadata/:seedID/:seedMethod", this.GetMetadata)
	m.Get("/api/abort-seed-stage/:seedID/:seedStage", this.AbortSeedStage)
	m.Get("/api/seed-stage-state/:seedID/:seedStage", this.SeedStageState)
	m.Get("/api/active-seed", this.ActiveSeed)
	m.Get("/api/post-seed-cmd/:seedID", this.postSeedCmd)

	// unused
	m.Get("/api/mysql-binlog-binary-contents", this.BinlogBinaryContents)
	m.Get("/api/mysql-relay-log-index-file", this.RelayLogIndexFile)
	m.Get("/api/mysql-relay-log-files", this.RelayLogFiles)
	m.Get("/api/mysql-relay-log-end-coordinates", this.RelayLogEndCoordinates)
	m.Get("/api/mysql-binlog-contents", this.BinlogContents)

	// called by orchestrator but never used
	m.Get("/api/mysql-relaylog-contents-tail/:relaylog/:start", this.RelaylogContentsTail)
	m.Post("/api/apply-relaylog-contents", this.ApplyRelaylogContents)

	m.Get("/api/custom-commands/:cmd", this.RunCommand)

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
