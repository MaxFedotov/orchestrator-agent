package osagent

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/github/orchestrator-agent/go/helper/cmd"
)

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

func getLogicalVolumePath(volumeName string, execWithSudo bool) (string, error) {
	if logicalVolumes, err := GetLogicalVolumes(volumeName, "", execWithSudo); err == nil && len(logicalVolumes) > 0 {
		return logicalVolumes[0].Path, err
	}
	return "", fmt.Errorf("Logical volume not found: %+v", volumeName)
}

func getLogicalVolumeFSType(volumeName string, execWithSudo bool) (string, error) {
	output, err := cmd.CommandOutput(fmt.Sprintf("blkid %s", volumeName), execWithSudo)
	if err != nil {
		return "", err
	}
	lines := cmd.OutputLines(output)
	re := regexp.MustCompile(`TYPE="(.*?)"`)
	for _, line := range lines {
		fsType := re.FindStringSubmatch(line)[1]
		return fsType, nil
	}
	return "", fmt.Errorf("Cannot find FS type for logical volume %s", volumeName)
}

func GetSnapshotHosts(snapshotHostsCmd string, execWithSudo bool) ([]string, error) {
	output, err := cmd.CommandOutput(snapshotHostsCmd, execWithSudo)
	if err != nil {
		return nil, err
	}
	hosts := cmd.OutputLines(output)
	return hosts, nil
}

func GetLogicalVolumes(volumeName string, filterPattern string, execWithSudo bool) ([]*LogicalVolume, error) {
	logicalVolumes := []*LogicalVolume{}
	output, err := cmd.CommandOutput(fmt.Sprintf("lvs --noheading -o lv_name,vg_name,lv_path,snap_percent %s", volumeName), execWithSudo)
	if err != nil {
		return logicalVolumes, err
	}
	tokens := cmd.OutputTokens(`[ \t]+`, output)
	if len(tokens[0][0]) > 0 {
		for _, lineTokens := range tokens {
			logicalVolume := &LogicalVolume{
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
	}
	return logicalVolumes, nil
}

func GetMount(mountPoint string, execWithSudo bool) (*Mount, error) {
	mount := &Mount{
		Path:      mountPoint,
		IsMounted: false,
	}

	output, err := cmd.CommandOutput(fmt.Sprintf("grep %s /etc/mtab", mountPoint), execWithSudo)
	if err != nil {
		// when grep does not find rows, it returns an error. So this is actually OK
		return mount, fmt.Errorf("Mount point %s not found", mountPoint)
	}
	tokens := cmd.OutputTokens(`[ \t]+`, output)
	for _, lineTokens := range tokens {
		mount.IsMounted = true
		mount.Device = lineTokens[0]
		mount.Path = lineTokens[1]
		mount.FileSystem = lineTokens[2]
		mount.LVPath, _ = getLogicalVolumePath(mount.Device, execWithSudo)
		mount.DiskUsage, _ = GetDiskUsage(mountPoint, execWithSudo)
	}
	return mount, nil
}

func MountLV(mountPoint string, volumeName string, execWithSudo bool) (*Mount, error) {
	mount := &Mount{
		Path:      mountPoint,
		IsMounted: false,
	}
	if volumeName == "" {
		return mount, fmt.Errorf("Unable to mount logical volume: empty volumeName")
	}
	fsType, err := getLogicalVolumeFSType(volumeName, execWithSudo)
	if err != nil {
		return mount, fmt.Errorf("Unable to get logical file system type: %+v", err)
	}

	mountOptions := ""
	if fsType == "xfs" {
		mountOptions = "-o nouuid"
	}
	err = cmd.CommandRun(fmt.Sprintf("mount %s %s %s", mountOptions, volumeName, mountPoint), execWithSudo)
	if err != nil {
		return mount, err
	}

	return GetMount(mountPoint, execWithSudo)
}

func Unmount(mountPoint string, execWithSudo bool) (*Mount, error) {
	mount := &Mount{
		Path:      mountPoint,
		IsMounted: false,
	}
	err := cmd.CommandRun(fmt.Sprintf("umount %s", mountPoint), execWithSudo)
	if err != nil {
		return mount, err
	}
	return GetMount(mountPoint, execWithSudo)
}

func RemoveLV(volumeName string, execWithSudo bool) error {
	return cmd.CommandRun(fmt.Sprintf("lvremove --force %s", volumeName), execWithSudo)
}

func CreateSnapshot(createSnapshostCommand string) error {
	return cmd.CommandRun(createSnapshostCommand, false)
}
