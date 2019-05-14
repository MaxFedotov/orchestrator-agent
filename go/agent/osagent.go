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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/github/orchestrator-agent/go/config"
	"github.com/github/orchestrator-agent/go/helper/functions"
)

func init() {
	osPath := os.Getenv("PATH")
	os.Setenv("PATH", fmt.Sprintf("%s:/usr/sbin:/usr/bin:/sbin:/bin", osPath))
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
	output, err := functions.CommandOutput(command)
	hosts, err := functions.OutputLines(output, err)
	if err != nil {
		return []string{}, fmt.Errorf("%s %+v", errorMessage, err)
	}

	return hosts, nil
}

func logicalVolumes(volumeName string, filterPattern string) ([]LogicalVolume, error) {
	logicalVolumes := []LogicalVolume{}
	output, err := functions.CommandOutput(functions.SudoCmd(fmt.Sprintf(" lvs --noheading -o lv_name,vg_name,lv_path,snap_percent,time --sort -time %s", volumeName)))
	tokens, err := functions.OutputTokens(`[ \t]+`, output, err)
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
		logicalVolume.SnapshotDate, err = time.Parse("2019-04-15 13:08:56 +0000", lineTokens[5])
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

	output, err := functions.CommandOutput(fmt.Sprintf("grep %s /etc/mtab", mountPoint))
	tokens, err := functions.OutputTokens(`[ \t]+`, output, err)
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
	_, err := functions.CommandOutput(config.Config.MySQLServiceStatusCommand)
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
	output, err := functions.CommandOutput(functions.SudoCmd(fmt.Sprintf("tail -n 20 %s", errorLogPath)))
	tail, err := functions.OutputLines(output, err)
	return tail, err
}
