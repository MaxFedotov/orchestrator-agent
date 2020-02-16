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
	"strconv"
	"syscall"

	"github.com/github/orchestrator-agent/go/helper/cmd"
	"github.com/outbrain/golib/log"
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

/* OLD FUNCTIONS */

// PostCopy executes a post-copy command -- after LVM copy is done, before service starts. Some cleanup may go here.
func PostCopy(command string, execWithSudo bool) error {
	return cmd.CommandRun(command, execWithSudo)
}

func MySQLStop(execWithSudo bool) error {
	return cmd.CommandRun("systemctl stop mysqld", execWithSudo)
}

func MySQLStart(execWithSudo bool) error {
	return cmd.CommandRun("systemctl start mysqld", execWithSudo)
}

func SeedCommandCompleted(seedId string) bool {
	return false
}

func SeedCommandSucceeded(seedId string) bool {
	return false
}

func AbortSeed(seedId string) error {
	if cmd, ok := activeCommands[seedId]; ok {
		//log.Debugf("Killing process %d", cmd.Process.Pid)
		cmd.Kill()
	} else {
		log.Debug("Not killing: Process not found")
	}
	return nil
}

/*func GetMySQLDataDir() (string, error) {
	command := config.Config.MySQLDatadirCommand
	output, err := commandOutput(command)
	return strings.TrimSpace(fmt.Sprintf("%s", output)), err
}
*/

/*
func GetMySQLPort() (int64, error) {
	command := config.Config.MySQLPortCommand
	output, err := commandOutput(command)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(fmt.Sprintf("%s", output)), 10, 0)
}
*/

/*
func DiskUsage(path string) (int64, error) {
	var result int64

	output, err := commandOutput(sudoCmd(fmt.Sprintf("du -sb %s", path)))
	tokens, err := outputTokens(`[ \t]+`, output, err)
	if err != nil {
		return result, err
	}

	for _, lineTokens := range tokens {
		result, err = strconv.ParseInt(lineTokens[0], 10, 0)
		return result, err
	}
	return result, err
}
*/

/*

func ExecCustomCmdWithOutput(commandKey string) ([]byte, error) {
	return commandOutput(config.Config.CustomCommands[commandKey])
}

// MOVED TO helper/cmd/cmd.go

func commandSplit(commandText string) (string, []string) {
	tokens := regexp.MustCompile(`[ ]+`).Split(strings.TrimSpace(commandText), -1)
	return tokens[0], tokens[1:]
}

func execCmd(commandText string) (*exec.Cmd, string, error) {
	commandBytes := []byte(commandText)
	tmpFile, err := ioutil.TempFile("", "orchestrator-agent-cmd-")
	if err != nil {
		return nil, "", log.Errore(err)
	}
	ioutil.WriteFile(tmpFile.Name(), commandBytes, 0644)
	log.Debugf("execCmd: %s", commandText)
	return exec.Command("bash", tmpFile.Name()), tmpFile.Name(), nil
}

// Add sudo to a command if we're configured to do so.  Otherwise just a signifier of a
// privileged command
func sudoCmd(commandText string) string {
	if config.Config.ExecWithSudo {
		return "sudo " + commandText
	}
	return commandText
}

// commandOutput executes a command and return output bytes
func commandOutput(commandText string) ([]byte, error) {
	cmd, tmpFileName, err := execCmd(commandText)
	if err != nil {
		return nil, log.Errore(err)
	}
	defer os.Remove(tmpFileName)

	outputBytes, err := cmd.Output()
	if err != nil {
		return nil, log.Errore(err)
	}

	return outputBytes, nil
}

// commandRun executes a command
func commandRun(commandText string, onCommand func(*exec.Cmd)) error {
	cmd, tmpFileName, err := execCmd(commandText)
	if err != nil {
		return log.Errore(err)
	}
	defer os.Remove(tmpFileName)
	onCommand(cmd)

	err = cmd.Run()
	if err != nil {
		return log.Errore(err)
	}

	return nil
}

func outputLines(commandOutput []byte, err error) ([]string, error) {
	if err != nil {
		return nil, err
	}
	text := strings.Trim(fmt.Sprintf("%s", commandOutput), "\n")
	lines := strings.Split(text, "\n")
	return lines, err
}

func outputTokens(delimiterPattern string, commandOutput []byte, err error) ([][]string, error) {
	lines, err := outputLines(commandOutput, err)
	if err != nil {
		return nil, err
	}
	tokens := make([][]string, len(lines))
	for i := range tokens {
		tokens[i] = regexp.MustCompile(delimiterPattern).Split(lines[i], -1)
	}
	return tokens, err
}

// MOVED TO lvm.go
/*
func AvailableSnapshots(requireLocal bool) ([]string, error) {
	var command string
	if requireLocal {
		command = config.Config.AvailableLocalSnapshotHostsCommand
	} else {
		command = config.Config.AvailableSnapshotHostsCommand
	}
	output, err := commandOutput(command)
	hosts, err := outputLines(output, err)
	return hosts, err
}
*/

// DeleteMySQLDataDir self explanatory. Be responsible! This function does not verify the MySQL service is down
/*func DeleteMySQLDataDir() error {

	directory, err := GetMySQLDataDir()
	if err != nil {
		return err
	}

	directory = strings.TrimSpace(directory)
	if directory == "" {
		return errors.New("refusing to delete empty directory")
	}
	if path.Dir(directory) == directory {
		return errors.New(fmt.Sprintf("Directory %s seems to be root; refusing to delete", directory))
	}
	_, err = commandOutput(config.Config.MySQLDeleteDatadirContentCommand)

	return err
} */

/* func GetMySQLDataDirAvailableDiskSpace() (int64, error) {
	directory, err := GetMySQLDataDir()
	if err != nil {
		return 0, log.Errore(err)
	}

	output, err := commandOutput(fmt.Sprintf("df -PT -B 1 %s | sed -e /^Filesystem/d", directory))
	if err != nil {
		return 0, log.Errore(err)
	}

	if len(output) > 0 {
		tokens, err := outputTokens(`[ \t]+`, output, err)
		if err != nil {
			return 0, log.Errore(err)
		}
		for _, lineTokens := range tokens {
			result, err := strconv.ParseInt(lineTokens[4], 10, 0)
			return result, err
		}
	}
	return 0, log.Errore(errors.New(fmt.Sprintf("No rows found by df in GetMySQLDataDirAvailableDiskSpace, %s", directory)))
}
*/

/* func HeuristicMySQLDataPath(mountPoint string) (string, error) {
	datadir, err := GetMySQLDataDir()
	if err != nil {
		return "", err
	}

	heuristicFileName := "ibdata1"

	re := regexp.MustCompile(`/[^/]+(.*)`)
	for {
		heuristicFullPath := path.Join(mountPoint, datadir, heuristicFileName)
		log.Debugf("search for %s", heuristicFullPath)
		if _, err := os.Stat(heuristicFullPath); err == nil {
			return path.Join(mountPoint, datadir), nil
		}
		if datadir == "" {
			return "", errors.New("Cannot detect MySQL datadir")
		}
		datadir = re.FindStringSubmatch(datadir)[1]
	}
}
*/
