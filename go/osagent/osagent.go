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

package osagent

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"syscall"

	"github.com/github/orchestrator-agent/go/helper/cmd"
	"gopkg.in/pipe.v2"
)

// DiskStat describes availiable disk statistic methods
type FSStat int

const (
	// Total folder size
	Total FSStat = iota
	// Free folder space
	Free
	// Used folder space
	Used
)

const (
	SeedTransferPort = 21234
)

var activeCommands = make(map[string]*pipe.State)

func init() {
	osPath := os.Getenv("PATH")
	os.Setenv("PATH", fmt.Sprintf("%s:/usr/sbin:/usr/bin:/sbin:/bin", osPath))
}

// GetFSStatistics returns different filesystem stats for path according to stat parameter - total\free\used size
func GetFSStatistics(path string, stat FSStat) (int64, error) {
	var size int64

	fs := syscall.Statfs_t{}
	err := syscall.Statfs(path, &fs)
	if err != nil {
		return size, err
	}
	switch stat {
	case Total:
		size = int64(fs.Blocks) * int64(fs.Bsize)
	case Free:
		size = int64(fs.Bavail) * int64(fs.Bsize)
	case Used:
		size = (int64(fs.Blocks) * int64(fs.Bsize)) - (int64(fs.Bavail) * int64(fs.Bsize))
	default:
		size = (int64(fs.Blocks) * int64(fs.Bsize)) - (int64(fs.Bavail) * int64(fs.Bsize))
	}
	return size, err
}

// GetDiskUsage returns disk usage for specified path
func GetDiskUsage(path string, cmd *cmd.CmdOpts) (int64, error) {
	var result int64

	output, err := cmd.CommandOutput(fmt.Sprintf("du -sb %s", path))
	if err != nil {
		return result, err
	}
	tokens := cmd.OutputTokens(`[ \t]+`, output)
	for _, lineTokens := range tokens {
		result, err = strconv.ParseInt(lineTokens[0], 10, 0)
		return result, err
	}
	return result, err
}

// MySQLErrorLogTail returns last 40 lines of MySQL error log
func GetMySQLErrorLogTail(logFile string, cmd *cmd.CmdOpts) (string, error) {
	output, err := cmd.CommandOutput(fmt.Sprintf("tail -n 40 %s", logFile))
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func CheckPermissionsOnFolder(folder string, cmd *cmd.CmdOpts) error {
	return cmd.CommandRun(fmt.Sprintf("touch %s -c", path.Join(folder, "orch-perm-test")))
}

// MySQLRunning checks if mysql is running
func MySQLRunning(command string, cmd *cmd.CmdOpts) (bool, error) {
	_, err := cmd.CommandOutput(command)
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, fmt.Errorf("failed to run systemctl: %v", err)
	}
	return true, nil
}

func MySQLStop(command string, cmd *cmd.CmdOpts) error {
	return cmd.CommandRun(command)
}

func MySQLStart(command string, cmd *cmd.CmdOpts) error {
	return cmd.CommandRun(command)
}

// GetOSName returns OS name
func GetOSName(cmd *cmd.CmdOpts) (string, error) {
	output, err := cmd.CommandOutput("hostnamectl | grep 'Operating System' | awk -F \":\" '{print $NF}'")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), err
}

// GetMemoryTotal returns total availiable system memory
func GetMemoryTotal(cmd *cmd.CmdOpts) (int64, error) {
	output, err := cmd.CommandOutput("grep MemTotal /proc/meminfo | awk '{print $2}'")
	if err != nil {
		return 0, err
	}
	mem, err := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64)
	if err != nil {
		return 0, err
	}
	return mem * 1024, err
}
