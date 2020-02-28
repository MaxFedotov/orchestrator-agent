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

/* NEW FUNCTIONS */

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

// MySQLRunning checks if mysql is running
func MySQLRunning(execWithSudo bool) (bool, error) {
	_, err := cmd.CommandOutput("systemctl check mysqld", execWithSudo)
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, fmt.Errorf("failed to run systemctl: %v", err)
	}
	return true, nil
}

// GetDiskUsage returns disk usage for specified path
func GetDiskUsage(path string, execWithSudo bool) (int64, error) {
	var result int64

	output, err := cmd.CommandOutput(fmt.Sprintf("du -sb %s", path), execWithSudo)
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

// MySQLErrorLogTail returns last 20 lines of MySQL error log
func GetMySQLErrorLogTail(logFile string, execWithSudo bool) ([]string, error) {
	output, err := cmd.CommandOutput(fmt.Sprintf("tail -n 20 %s", logFile), execWithSudo)
	if err != nil {
		return nil, err
	}
	tail := cmd.OutputLines(output)
	return tail, nil
}

func CheckPermissionsOnFolder(folder string, execWithSudo bool) error {
	return cmd.CommandRun(fmt.Sprintf("touch %s -c", path.Join(folder, "orch-perm-test")), execWithSudo)
}

func MySQLStop(execWithSudo bool) error {
	return cmd.CommandRun("systemctl stop mysqld", execWithSudo)
}

func MySQLStart(execWithSudo bool) error {
	return cmd.CommandRun("systemctl start mysqld", execWithSudo)
}
