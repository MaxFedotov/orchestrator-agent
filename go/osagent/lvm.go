package osagent

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/github/orchestrator-agent/go/helper/cmd"
)

// LogicalVolume describes an LVM volume
type LogicalVolume struct {
	Name            string
	GroupName       string
	Path            string
	IsSnapshot      bool
	SnapshotPercent float64
	CreatedAt       time.Time
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

func getLogicalVolumePath(volumeName string, cmd *cmd.CmdOpts) (string, error) {
	if logicalVolumes, err := GetLogicalVolumes(volumeName, "", cmd); err == nil && len(logicalVolumes) > 0 {
		return logicalVolumes[0].Path, err
	}
	return "", fmt.Errorf("Logical volume not found: %+v", volumeName)
}

func getLogicalVolumeFSType(volumeName string, cmd *cmd.CmdOpts) (string, error) {
	output, err := cmd.CommandOutput(fmt.Sprintf("blkid %s", volumeName))
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

func GetSnapshotHosts(snapshotHostsCmd string, cmd *cmd.CmdOpts) ([]string, error) {
	if len(snapshotHostsCmd) == 0 {
		return []string{}, nil
	}
	output, err := cmd.CommandOutput(snapshotHostsCmd)
	if err != nil {
		return []string{}, err
	}
	hosts := cmd.OutputLines(output)
	return hosts, nil
}

func GetLogicalVolumes(volumeName string, filterPattern string, cmd *cmd.CmdOpts) ([]*LogicalVolume, error) {
	var createdAt time.Time
	logicalVolumes := []*LogicalVolume{}
	output, err := cmd.CommandOutput(fmt.Sprintf("lvs --noheading -o lv_name,vg_name,lv_path,snap_percent %s", volumeName))
	if err != nil {
		return logicalVolumes, err
	}
	tokens := cmd.OutputTokens(`[ \t]+`, output)
	if len(tokens) > 0 {
		for _, lineTokens := range tokens {
			if len(lineTokens) > 4 {
				logicalVolume := &LogicalVolume{
					Name:      lineTokens[1],
					GroupName: lineTokens[2],
					Path:      lineTokens[3],
				}
				logicalVolume.SnapshotPercent, err = strconv.ParseFloat(lineTokens[4], 32)
				logicalVolume.IsSnapshot = (err == nil)
				if strings.Contains(logicalVolume.Name, filterPattern) {
					output, err := cmd.CommandOutput(fmt.Sprintf("lvdisplay %s | grep -e 'LV Creation host, time' | awk '{print $6\" \"$7}'", logicalVolume.Path))
					if err == nil {
						createdAt, err = time.Parse("2006-01-02 15:04:05", cmd.OutputLines(output)[0])
						if err == nil {
							logicalVolume.CreatedAt = createdAt
						}
					}
					logicalVolumes = append(logicalVolumes, logicalVolume)
				}
			}
		}
	}
	return logicalVolumes, nil
}

func GetMount(mountPoint string, cmd *cmd.CmdOpts) (*Mount, error) {
	mount := &Mount{
		Path:      mountPoint,
		IsMounted: false,
	}

	output, err := cmd.CommandOutput(fmt.Sprintf("grep %s /etc/mtab", mountPoint))
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
		mount.LVPath, _ = getLogicalVolumePath(mount.Device, cmd)
		mount.DiskUsage, _ = GetDiskUsage(mountPoint, cmd)
	}
	return mount, nil
}

func MountLV(mountPoint string, volumeName string, cmd *cmd.CmdOpts) (*Mount, error) {
	mount := &Mount{
		Path:      mountPoint,
		IsMounted: false,
	}
	if volumeName == "" {
		return mount, fmt.Errorf("Unable to mount logical volume: empty volumeName")
	}
	fsType, err := getLogicalVolumeFSType(volumeName, cmd)
	if err != nil {
		return mount, fmt.Errorf("Unable to get logical file system type: %+v", err)
	}

	mountOptions := ""
	if fsType == "xfs" {
		mountOptions = "-o nouuid"
	}
	err = cmd.CommandRun(fmt.Sprintf("mount %s %s %s", mountOptions, volumeName, mountPoint))
	if err != nil {
		return mount, err
	}

	return GetMount(mountPoint, cmd)
}

func Unmount(mountPoint string, cmd *cmd.CmdOpts) error {
	return cmd.CommandRun(fmt.Sprintf("umount %s", mountPoint))
}

func RemoveLV(volumeName string, cmd *cmd.CmdOpts) error {
	return cmd.CommandRun(fmt.Sprintf("lvremove --force %s", volumeName))
}

func CreateSnapshot(createSnapshostCommand string, cmd *cmd.CmdOpts) error {
	return cmd.CommandRun(createSnapshostCommand)
}
