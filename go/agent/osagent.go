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
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/github/orchestrator-agent/go/config"
	"github.com/openark/golib/log"
)

// Add sudo to a command if we're configured to do so.  Otherwise just a signifier of a
// privileged command
func sudoCmd(commandText string) string {
	if config.Config.ExecWithSudo {
		return "sudo " + commandText
	}
	return commandText
}

func execCmd(commandText string) (*exec.Cmd, string, error) {
	commandBytes := []byte(commandText)
	tmpFile, err := ioutil.TempFile("", "orchestrator-agent-cmd-")
	if err != nil {
		return nil, "", err
	}
	ioutil.WriteFile(tmpFile.Name(), commandBytes, 0644)
	log.Debugf("execCmd: %s", commandText)
	return exec.Command("bash", tmpFile.Name()), tmpFile.Name(), nil
}

// commandOutput executes a command and return output bytes
func commandOutput(commandText string) ([]byte, error) {
	cmd, tmpFileName, err := execCmd(commandText)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFileName)

	outputBytes, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error executing command '%s' - %+v", commandText, err)
	}

	return outputBytes, nil
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

func availableSnapshots(requireLocal bool) ([]string, error) {
	var command string
	var errorMessage string
	if requireLocal {
		command = config.Config.AvailableLocalSnapshotHostsCommand
		errorMessage = "Unable to get local snapshots info:"
	} else {
		command = config.Config.AvailableSnapshotHostsCommand
		errorMessage = "Unable to get snapshots info:"
	}
	output, err := commandOutput(command)
	hosts, err := outputLines(output, err)
	if err != nil {
		return []string{}, fmt.Errorf("%s %+v", errorMessage, err)
	}

	return hosts, nil
}

func logicalVolumes(volumeName string, filterPattern string) ([]LogicalVolume, error) {
	logicalVolumes := []LogicalVolume{}
	output, err := commandOutput(sudoCmd(fmt.Sprintf("lvs --noheading -o lv_name,vg_name,lv_path,snap_percent %s", volumeName)))
	tokens, err := outputTokens(`[ \t]+`, output, err)
	if err != nil {
		return logicalVolumes, fmt.Errorf("Unable to get logical volumes info: %+v", err)
	}
	for _, lineTokens := range tokens {
		logicalVolume := LogicalVolume{
			Name:      lineTokens[1],
			GroupName: lineTokens[2],
			Path:      lineTokens[3],
		}
		logicalVolume.SnapshotPercent, err = strconv.ParseFloat(lineTokens[4], 32)
		logicalVolume.IsSnapshot = (err == nil)
		if strings.Contains(logicalVolume.Name, filterPattern) {
			logicalVolumes = append(logicalVolumes, logicalVolume)
		}
	}
	return logicalVolumes, err
}

func getMount(mountPoint string) Mount {
	mount := Mount{
		Path:      mountPoint,
		IsMounted: false,
	}

	output, err := commandOutput(fmt.Sprintf("grep %s /etc/mtab", mountPoint))
	tokens, err := outputTokens(`[ \t]+`, output, err)
	if err != nil {
		// when grep does not find rows, it returns an error. So this is actually OK
		return mount
	}

	for _, lineTokens := range tokens {
		mount.IsMounted = true
		mount.Device = lineTokens[0]
		mount.Path = lineTokens[1]
		mount.FileSystem = lineTokens[2]
		mount.LVPath, _ = getLogicalVolumePath(mount.Device)
		mount.DiskUsage, _ = getDiskStatistics(mountPoint, "used")
	}
	return mount
}

func getLogicalVolumePath(volumeName string) (string, error) {
	if logicalVolumes, err := logicalVolumes(volumeName, ""); err == nil && len(logicalVolumes) > 0 {
		return logicalVolumes[0].Path, err
	}
	return "", fmt.Errorf("logical volume not found: %+v", volumeName)
}

func mySQLRunning() bool {
	_, err := commandOutput(config.Config.MySQLServiceStatusCommand)
	// status command exits with 0 when MySQL is running, or otherwise if not running
	return err == nil
}

func getDiskStatistics(path string, stat string) (int64, error) {
	var size int64

	fs := syscall.Statfs_t{}
	err := syscall.Statfs(path, &fs)
	if err != nil {
		return size, err
	}
	switch stat {
	case "total":
		size = int64(fs.Blocks) * int64(fs.Bsize)
	case "free":
		size = int64(fs.Bavail) * int64(fs.Bsize)
	case "used":
		size = (int64(fs.Blocks) * int64(fs.Bsize)) - (int64(fs.Bavail) * int64(fs.Bsize))
	default:
		size = (int64(fs.Blocks) * int64(fs.Bsize)) - (int64(fs.Bavail) * int64(fs.Bsize))
	}
	return size, err
}

func getDirectorySize(path string) (int64, error) {
	if path == "" {
		return 0, errors.New("Unable to calculate size of empty directory")
	}
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})
	return size, err
}

func mySQLErrorLogTail(errorLogPath string) ([]string, error) {
	output, err := commandOutput(sudoCmd(fmt.Sprintf("tail -n 20 %s", errorLogPath)))
	tail, err := outputLines(output, err)
	return tail, err
}
