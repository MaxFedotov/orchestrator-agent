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
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/github/orchestrator-agent/go/config"
	"github.com/github/orchestrator-agent/go/dbagent"
	"github.com/github/orchestrator-agent/go/inst"
	"github.com/openark/golib/log"
)

var activeCommands = make(map[string]*exec.Cmd)
var seedMethods = []string{"xtrabackup", "xtrabackup-stream", "lvm", "mydumper", "mysqldump"}

const (
	mysqlbackupFileName           = "backup.sql"
	mysqlbackupCompressedFileName = "backup.sql.gz"
	mydumperMetadataFile          = "metadata"
)

// LogicalVolume describes an LVM volume
type LogicalVolume struct {
	Name            string
	GroupName       string
	Path            string
	IsSnapshot      bool
	SnapshotPercent float64
}

// MySQLDatabaseInfo provides information about MySQL databases, engines and sizes
type MySQLDatabaseInfo struct {
	MySQLDatabases map[string]*MySQLDatabase
	InnoDBLogSize  int64
}

// MySQLDatabase info provides information about MySQL databases, engines and sizes
type MySQLDatabase struct {
	Engines      []string
	PhysicalSize int64
	LogicalSize  int64
}

// GetRelayLogIndexFileName attempts to find the relay log index file under the mysql datadir
func GetRelayLogIndexFileName() (string, error) {
	directory := config.Config.MySQLDataDir
	output, err := commandOutput(fmt.Sprintf("ls %s/*relay*.index", directory))
	if err != nil {
		return "", log.Errore(err)
	}

	return strings.TrimSpace(fmt.Sprintf("%s", output)), err
}

// GetRelayLogFileNames attempts to find the active relay logs
func GetRelayLogFileNames() (fileNames []string, err error) {
	relayLogIndexFile, err := GetRelayLogIndexFileName()
	if err != nil {
		return fileNames, log.Errore(err)
	}

	contents, err := ioutil.ReadFile(relayLogIndexFile)
	if err != nil {
		return fileNames, log.Errore(err)
	}

	for _, fileName := range strings.Split(string(contents), "\n") {
		if fileName != "" {
			fileName = path.Join(path.Dir(relayLogIndexFile), fileName)
			fileNames = append(fileNames, fileName)
		}
	}
	return fileNames, nil
}

// GetRelayLogEndCoordinates returns the coordinates at the end of relay logs
func GetRelayLogEndCoordinates() (coordinates *inst.BinlogCoordinates, err error) {
	relaylogFileNames, err := GetRelayLogFileNames()
	if err != nil {
		return coordinates, log.Errore(err)
	}

	lastRelayLogFile := relaylogFileNames[len(relaylogFileNames)-1]
	output, err := commandOutput(sudoCmd(fmt.Sprintf("du -b %s", lastRelayLogFile)))
	tokens, err := outputTokens(`[ \t]+`, output, err)
	if err != nil {
		return coordinates, err
	}

	var fileSize int64
	for _, lineTokens := range tokens {
		fileSize, err = strconv.ParseInt(lineTokens[0], 10, 0)
	}
	if err != nil {
		return coordinates, err
	}
	return &inst.BinlogCoordinates{LogFile: lastRelayLogFile, LogPos: fileSize, Type: inst.RelayLog}, nil
}

func MySQLBinlogContents(binlogFiles []string, startPosition int64, stopPosition int64) (string, error) {
	if len(binlogFiles) == 0 {
		return "", log.Errorf("No binlog files provided in MySQLBinlogContents")
	}
	cmd := `mysqlbinlog`
	for _, binlogFile := range binlogFiles {
		cmd = fmt.Sprintf("%s %s", cmd, binlogFile)
	}
	if startPosition != 0 {
		cmd = fmt.Sprintf("%s --start-position=%d", cmd, startPosition)
	}
	if stopPosition != 0 {
		cmd = fmt.Sprintf("%s --stop-position=%d", cmd, stopPosition)
	}
	cmd = fmt.Sprintf("%s | gzip | base64", cmd)

	output, err := commandOutput(cmd)
	return string(output), err
}

func MySQLBinlogContentHeaderSize(binlogFile string) (int64, error) {
	// magic header
	// There are the first 4 bytes, and then there's also the first entry (the format-description).
	// We need both from the first log file.
	// Typically, the format description ends at pos 120, but let's verify...

	cmd := fmt.Sprintf("mysqlbinlog %s --start-position=4 | head | egrep -o 'end_log_pos [^ ]+' | head -1 | awk '{print $2}'", binlogFile)
	if content, err := commandOutput(sudoCmd(cmd)); err != nil {
		return 0, err
	} else {
		return strconv.ParseInt(strings.TrimSpace(string(content)), 10, 0)
	}
}

func MySQLBinlogBinaryContents(binlogFiles []string, startPosition int64, stopPosition int64) (result string, err error) {
	if len(binlogFiles) == 0 {
		return "", log.Errorf("No binlog files provided in MySQLBinlogContents")
	}
	tmpFile, err := ioutil.TempFile("", "orchestrator-agent-binlog-contents-")
	if err != nil {
		return "", log.Errore(err)
	}
	var headerSize int64
	if startPosition != 0 {
		if headerSize, err = MySQLBinlogContentHeaderSize(binlogFiles[0]); err != nil {
			return "", log.Errore(err)
		}
		cmd := fmt.Sprintf("cat %s | head -c%d >> %s", binlogFiles[0], headerSize, tmpFile.Name())
		if _, err := commandOutput(sudoCmd(cmd)); err != nil {
			return "", err
		}
	}
	for i, binlogFile := range binlogFiles {
		cmd := fmt.Sprintf("cat %s", binlogFile)

		if i == len(binlogFiles)-1 && stopPosition != 0 {
			cmd = fmt.Sprintf("%s | head -c %d", cmd, stopPosition)
		}
		if i == 0 && startPosition != 0 {
			cmd = fmt.Sprintf("%s | tail -c+%d", cmd, startPosition+1)
		}
		if i > 0 {
			// At any case, we drop out binlog header (magic + format_description) for next relay logs
			if headerSize, err = MySQLBinlogContentHeaderSize(binlogFile); err != nil {
				return "", log.Errore(err)
			}
			cmd = fmt.Sprintf("%s | tail -c+%d", cmd, headerSize+1)
		}
		cmd = fmt.Sprintf("%s >> %s", cmd, tmpFile.Name())
		if _, err := commandOutput(sudoCmd(cmd)); err != nil {
			return "", err
		}
	}

	cmd := fmt.Sprintf("cat %s | gzip | base64", tmpFile.Name())
	output, err := commandOutput(cmd)
	return string(output), err
}

func ApplyRelaylogContents(content []byte) error {
	encodedContentsFile, err := ioutil.TempFile("", "orchestrator-agent-apply-relaylog-encoded-")
	if err != nil {
		return log.Errore(err)
	}
	if err := ioutil.WriteFile(encodedContentsFile.Name(), content, 0644); err != nil {
		return log.Errore(err)
	}

	relaylogContentsFile, err := ioutil.TempFile("", "orchestrator-agent-apply-relaylog-bin-")
	if err != nil {
		return log.Errore(err)
	}

	cmd := fmt.Sprintf("cat %s | base64 --decode | gunzip > %s", encodedContentsFile.Name(), relaylogContentsFile.Name())
	if _, err := commandOutput(sudoCmd(cmd)); err != nil {
		return log.Errore(err)
	}

	if config.Config.MySQLClientCommand != "" {
		cmd := fmt.Sprintf("mysqlbinlog %s | %s", relaylogContentsFile.Name(), config.Config.MySQLClientCommand)
		if _, err := commandOutput(sudoCmd(cmd)); err != nil {
			return log.Errore(err)
		}
	}
	log.Infof("Applied relay log contents from %s", relaylogContentsFile.Name())

	return nil
}

// Equals tests equality of this corrdinate and another one.
func (this *LogicalVolume) IsSnapshotValid() bool {
	if !this.IsSnapshot {
		return false
	}
	if this.SnapshotPercent >= 100.0 {
		return false
	}
	return true
}

// Mount describes a file system mount point
type Mount struct {
	Path           string
	Device         string
	LVPath         string
	FileSystem     string
	IsMounted      bool
	DiskUsage      int64
	MySQLDataPath  string
	MySQLDiskUsage int64
}

type DiskStats struct {
	Total int64
	Free  int64
	Used  int64
}

func init() {
	osPath := os.Getenv("PATH")
	os.Setenv("PATH", fmt.Sprintf("%s:/usr/sbin:/usr/bin:/sbin:/bin", osPath))
}

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
	var stderr bytes.Buffer
	cmd, tmpFileName, err := execCmd(commandText)
	cmd.Stderr = &stderr
	if err != nil {
		return log.Errore(err)
	}
	defer os.Remove(tmpFileName)
	onCommand(cmd)

	err = cmd.Run()
	if err != nil {
		return log.Errorf(stderr.String())
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

func Hostname() (string, error) {
	return os.Hostname()
}

func LogicalVolumes(volumeName string, filterPattern string) ([]LogicalVolume, error) {
	output, err := commandOutput(sudoCmd(fmt.Sprintf("lvs --noheading -o lv_name,vg_name,lv_path,snap_percent %s", volumeName)))
	tokens, err := outputTokens(`[ \t]+`, output, err)
	if err != nil {
		return nil, err
	}

	logicalVolumes := []LogicalVolume{}
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
	return logicalVolumes, nil
}

func GetLogicalVolumePath(volumeName string) (string, error) {
	if logicalVolumes, err := LogicalVolumes(volumeName, ""); err == nil && len(logicalVolumes) > 0 {
		return logicalVolumes[0].Path, err
	}
	return "", errors.New(fmt.Sprintf("logical volume not found: %+v", volumeName))
}

func GetLogicalVolumeFSType(volumeName string) (string, error) {
	command := fmt.Sprintf("blkid %s", volumeName)
	output, err := commandOutput(sudoCmd(command))
	lines, err := outputLines(output, err)
	re := regexp.MustCompile(`TYPE="(.*?)"`)
	for _, line := range lines {
		fsType := re.FindStringSubmatch(line)[1]
		return fsType, nil
	}
	return "", errors.New(fmt.Sprintf("Cannot find FS type for logical volume %s", volumeName))
}

func GetMount(mountPoint string) (Mount, error) {
	mount := Mount{
		Path:      mountPoint,
		IsMounted: false,
	}

	output, err := commandOutput(fmt.Sprintf("grep %s /etc/mtab", mountPoint))
	tokens, err := outputTokens(`[ \t]+`, output, err)
	if err != nil {
		// when grep does not find rows, it returns an error. So this is actually OK
		return mount, nil
	}

	for _, lineTokens := range tokens {
		mount.IsMounted = true
		mount.Device = lineTokens[0]
		mount.Path = lineTokens[1]
		mount.FileSystem = lineTokens[2]
		mount.LVPath, _ = GetLogicalVolumePath(mount.Device)
		mount.DiskUsage, _ = DiskUsed(mountPoint)
		mount.MySQLDataPath, _ = HeuristicMySQLDataPath(mountPoint)
		mount.MySQLDiskUsage, _ = DirectorySize(mount.MySQLDataPath)
	}
	return mount, nil
}

func MountLV(mountPoint string, volumeName string) (Mount, error) {
	mount := Mount{
		Path:      mountPoint,
		IsMounted: false,
	}
	if volumeName == "" {
		return mount, errors.New("Empty columeName in MountLV")
	}
	fsType, err := GetLogicalVolumeFSType(volumeName)
	if err != nil {
		return mount, err
	}

	mountOptions := ""
	if fsType == "xfs" {
		mountOptions = "-o nouuid"
	}
	_, err = commandOutput(sudoCmd(fmt.Sprintf("mount %s %s %s", mountOptions, volumeName, mountPoint)))
	if err != nil {
		return mount, err
	}

	return GetMount(mountPoint)
}

func RemoveLV(volumeName string) error {
	_, err := commandOutput(sudoCmd(fmt.Sprintf("lvremove --force %s", volumeName)))
	return err
}

func CreateSnapshot() error {
	_, err := commandOutput(config.Config.CreateSnapshotCommand)
	return err
}

func Unmount(mountPoint string) (Mount, error) {
	mount := Mount{
		Path:      mountPoint,
		IsMounted: false,
	}
	_, err := commandOutput(sudoCmd(fmt.Sprintf("umount %s", mountPoint)))
	if err != nil {
		return mount, err
	}
	return GetMount(mountPoint)
}

func GetDiskSpace(path string) (DiskStats, error) {
	disk := DiskStats{}
	fs := syscall.Statfs_t{}
	err := syscall.Statfs(path, &fs)
	if err != nil {
		return disk, err
	}
	disk.Total = int64(fs.Blocks) * int64(fs.Bsize)
	disk.Free = int64(fs.Bavail) * int64(fs.Bsize)
	disk.Used = disk.Total - disk.Free

	return disk, err
}

func DiskUsed(path string) (int64, error) {
	du, err := GetDiskSpace(path)
	return du.Used, err
}

func DiskFree(path string) (int64, error) {
	du, err := GetDiskSpace(path)
	return du.Free, err
}

func DirectorySize(path string) (int64, error) {
	if path == "" {
		return 0, errors.New("cannot calculate size of empty directory")
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

// DeleteDirContents deletes all contens on underlying path
func DeleteDirContents(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	names, err := dir.Readdirnames(-1)
	if err != nil {
		return err
	}
	for _, name := range names {
		err = os.RemoveAll(filepath.Join(path, name))
		if err != nil {
			return err
		}
	}
	return nil
}

// DeleteFile deletes file located in folder. Can be used with wildcards
func DeleteFile(path string, file string) error {
	files, err := filepath.Glob(filepath.Join(path, file))
	if err != nil {
		return log.Errore(err)
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil {
			return log.Errore(err)
		}
	}
	return err
}

// PostCopy executes a post-copy command -- after LVM copy is done, before service starts. Some cleanup may go here.
func PostCopy() error {
	_, err := commandOutput(config.Config.PostCopyCommand)
	return err
}

func HeuristicMySQLDataPath(mountPoint string) (string, error) {
	datadir := config.Config.MySQLDataDir

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

func MySQLErrorLogTail() ([]string, error) {
	lines := 20
	var bufsize int64 = 400
	var result []string
	var iter int64

	file, err := os.Open(config.Config.MySQLErrorLog)
	fstat, err := file.Stat()
	defer file.Close()
	if err != nil {
		return nil, err
	}
	fsize := fstat.Size()
	if bufsize > fsize {
		bufsize = fsize
	}
	reader := bufio.NewReader(file)
	for {
		iter++
		var data []string
		offset := fsize - bufsize*iter
		if offset < 0 {
			offset = 0
		}
		currentPosition, _ := file.Seek(offset, 0)
		for {
			line, err := reader.ReadString('\n')
			data = append(data, line)
			if err != nil {
				if err == io.EOF {
					break
				}
			}
		}
		data = data[:len(data)-1]
		if len(data) >= lines || currentPosition == 0 {
			if lines < len(data) {
				result = data[len(data)-lines:]
			} else {
				result = data
			}
			break
		}
	}
	return result, err
}

func MySQLRunning() (bool, error) {
	_, err := commandOutput(config.Config.MySQLServiceStatusCommand)
	// status command exits with 0 when MySQL is running, or otherwise if not running
	return err == nil, nil
}

func MySQLStop() error {
	_, err := commandOutput(config.Config.MySQLServiceStopCommand)
	return err
}

func MySQLStart() error {
	_, err := commandOutput(config.Config.MySQLServiceStartCommand)
	return err
}

func MySQLRestart() error {
	cmd := fmt.Sprintf("%s; sleep 20", config.Config.MySQLServiceRestartCommand)
	_, err := commandOutput(cmd)
	return err
}

func ReceiveMySQLSeedData(seedId string) error {
	directory := config.Config.MySQLDataDir

	if directory == "" {
		return log.Error("Empty directory in ReceiveMySQLSeedData")
	}

	err := commandRun(
		fmt.Sprintf("%s %s %d", config.Config.ReceiveSeedDataCommand, directory, config.Config.SeedPort),
		func(cmd *exec.Cmd) {
			activeCommands[seedId] = cmd
			log.Debug("ReceiveMySQLSeedData command completed")
		})
	if err != nil {
		return log.Errore(err)
	}

	return err
}

func SendMySQLSeedData(targetHostname string, directory string, seedId string) error {
	if directory == "" {
		return log.Error("Empty directory in SendMySQLSeedData")
	}
	err := commandRun(fmt.Sprintf("%s %s %s %d", config.Config.SendSeedDataCommand, directory, targetHostname, config.Config.SeedPort),
		func(cmd *exec.Cmd) {
			activeCommands[seedId] = cmd
			log.Debug("SendMySQLSeedData command completed")
		})
	if err != nil {
		return log.Errore(err)
	}
	return err
}

func SeedCommandCompleted(seedId string) bool {
	if cmd, ok := activeCommands[seedId]; ok {
		if cmd.ProcessState != nil {
			return cmd.ProcessState.Exited()
		}
	}
	return false
}

func SeedCommandSucceeded(seedId string) bool {
	if cmd, ok := activeCommands[seedId]; ok {
		if cmd.ProcessState != nil {
			return cmd.ProcessState.Success()
		}
	}
	return false
}

func AbortSeed(seedId string) error {
	if cmd, ok := activeCommands[seedId]; ok {
		log.Debugf("Killing process %d", cmd.Process.Pid)
		return cmd.Process.Kill()
	} else {
		log.Debug("Not killing: Process not found")
	}
	return nil
}

func ExecCustomCmdWithOutput(commandKey string) ([]byte, error) {
	return commandOutput(config.Config.CustomCommands[commandKey])
}

func contains(item string, list []string) bool {
	for _, b := range list {
		if item == b {
			return true
		}
	}
	return false
}

// These magical multiplies for logicalSize (0.6 in case of compression, 0.8 in other cases) are just raw estimates. They can be wrong, but we will use them
// as 'we should have at least' space check, because we can't make any accurate estimations for logical backups
func GetMySQLDatabaseInfo() (dbinfo MySQLDatabaseInfo, err error) {
	dbi := make(map[string]*MySQLDatabase)
	var physicalSize, tokuPhysicalSize, logicalSize int64 = 0, 0, 0
	databases, err := dbagent.GetMySQLDatabases()
	for _, db := range databases {
		engines, err := dbagent.GetMySQLEngines(db)
		if err != nil {
			log.Errore(err)
		}
		for _, engine := range engines {
			if engine != "TokuDB" {
				physicalSize, err = DirectorySize(path.Join(config.Config.MySQLDataDir, db))
				if err != nil {
					log.Errore(err)
				}
			} else {
				tokuPhysicalSize, err = dbagent.GetTokuDBSize(db)
				if err != nil {
					log.Errore(err)
				}
			}
		}
		physicalSize += tokuPhysicalSize
		if config.Config.CompressLogicalBackup {
			logicalSize = int64(float64(physicalSize) * 0.6)
		} else {
			logicalSize = int64(float64(physicalSize) * 0.8)
		}
		dbi[db] = &MySQLDatabase{engines, physicalSize, logicalSize}
		dbinfo.MySQLDatabases = dbi
	}
	dbinfo.InnoDBLogSize, err = dbagent.GetInnoDBLogSize()
	if err != nil {
		log.Errore(err)
	}
	return dbinfo, err
}

func GetAvailableSeedMethods() []string {
	var avaliableSeedMethods []string
	var cmd string

	for _, seedMethod := range seedMethods {
		switch seedMethod {
		case "lvm":
			cmd = "lvs"
		case "xtrabackup-stream":
			cmd = "xtrabackup"
		default:
			cmd = seedMethod
		}
		err := commandRun(
			fmt.Sprintf("%s --version", cmd),
			func(cmd *exec.Cmd) {
				log.Debug("Checking for seed method", seedMethod)
			})
		if err == nil {
			log.Debug("seed method", seedMethod, "found")
			avaliableSeedMethods = append(avaliableSeedMethods, seedMethod)
		}
	}
	return avaliableSeedMethods
}

func StartLocalBackup(seedId string, seedMethod string, databases string) (BackupFolder string, err error) {
	var cmd string
	BackupFolder = path.Join(config.Config.MySQLBackupDir, time.Now().Format("20060102-150405"))
	err = os.Mkdir(BackupFolder, 0755)
	if err != nil {
		return "", log.Errore(err)
	}
	if !contains(seedMethod, seedMethods) {
		return "", log.Errorf("Unsupported seed method")
	}
	if databases != "" {
		availiableDatabases, _ := dbagent.GetMySQLDatabases()
		for _, db := range strings.Split(databases, ",") {
			if !contains(db, availiableDatabases) {
				return "", log.Errorf("Cannot backup database %+v. Database doesn't exists", db)
			}
		}
	}
	err = dbagent.ManageReplicationUser()
	if err != nil {
		return "", log.Errore(err)
	}
	switch seedMethod {
	case "xtrabackup":
		cmd = fmt.Sprintf("xtrabackup --backup --user=%s --password=%s --port=%d --parallel=%d --target-dir=%s --databases='%s'",
			config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort, config.Config.XtrabackupParallelThreads, BackupFolder, strings.Replace(databases, ",", " ", -1))
	case "mydumper":
		cmd = fmt.Sprintf("mydumper --user=%s --password=%s --port=%d --threads=%d --outputdir=%s --triggers --events --routines --regex='(%s)'",
			config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort, config.Config.MyDumperParallelThreads, BackupFolder, strings.Replace(databases, ",", "\\.|", -1)+"\\.")
		if config.Config.CompressLogicalBackup {
			cmd += fmt.Sprintf(" --compress")
		}
		if config.Config.MyDumperRowsChunkSize != 0 {
			cmd += fmt.Sprintf(" --rows=%d", config.Config.MyDumperRowsChunkSize)
		}
	case "mysqldump":
		if databases == "" {
			cmd = fmt.Sprintf("mysqldump --user=%s --password=%s --port=%d --single-transaction --master-data=1 --routines --events --triggers --all-databases",
				config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort)
		} else {
			cmd = fmt.Sprintf("mysqldump --user=%s --password=%s --port=%d --single-transaction --master-data=1 --routines --events --triggers --databases %s",
				config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort, strings.Replace(databases, ",", " ", -1))
		}
		if config.Config.CompressLogicalBackup {
			cmd += fmt.Sprintf(" | gzip > %s", path.Join(BackupFolder, mysqlbackupCompressedFileName))
		} else {
			cmd += fmt.Sprintf(" > %s", path.Join(BackupFolder, mysqlbackupFileName))
		}
	}
	err = commandRun(
		fmt.Sprintf(cmd),
		func(cmd *exec.Cmd) {
			activeCommands[seedId] = cmd
			log.Debug("StartLocalBackup command completed")
		})
	if err != nil {
		return "", log.Errore(err)
	}
	return BackupFolder, err
}

func ReceiveBackup(seedId string, seedMethod string, backupFolder string) error {
	var cmd string
	var MySQLInnoDBLogDir string
	if !contains(seedMethod, seedMethods) {
		return log.Errorf("Unsupported seed method")
	}
	if backupFolder == config.Config.MySQLDataDir && seedMethod == "xtrabackup-stream" {
		if len(config.Config.MySQLInnoDBLogDir) == 0 {
			MySQLInnoDBLogDir = config.Config.MySQLDataDir
		} else {
			MySQLInnoDBLogDir = config.Config.MySQLInnoDBLogDir
		}
		cmd := fmt.Sprintf("mysqldump --user=%s --password=%s --port=%d --single-transaction --databases mysql  > %s/mysql_users_backup.sql",
			config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort, config.Config.MySQLBackupDir)
		err := commandRun(
			fmt.Sprintf(cmd),
			func(cmd *exec.Cmd) {
				activeCommands[seedId] = cmd
				log.Debug("Backing up MySQL users")
			})
		if err != nil {
			return log.Errore(err)
		}
		err = MySQLStop()
		if err != nil {
			return log.Errore(err)
		}
		err = DeleteFile(MySQLInnoDBLogDir, "ib_logfile*")
		if err != nil {
			return log.Errore(err)
		}
		err = DeleteDirContents(config.Config.MySQLDataDir)
		if err != nil {
			return log.Errore(err)
		}
	} else {
		err := os.Mkdir(backupFolder, 0755)
		if err != nil {
			return log.Errore(err)
		}
	}
	cmd = fmt.Sprintf("nc -l -w 20 -p %d ", config.Config.SeedPort)
	switch seedMethod {
	case "xtrabackup-stream":
		cmd += fmt.Sprintf("| xbstream -x -C %s", backupFolder)
	default:
		cmd += fmt.Sprintf("| tar xfz - -C %s", backupFolder)
	}
	err := commandRun(
		fmt.Sprintf(cmd),
		func(cmd *exec.Cmd) {
			activeCommands[seedId] = cmd
			log.Debug("Receiving MySQL backup")
		})
	if err != nil {
		return log.Errore(err)
	}
	return err
}

func SendLocalBackup(seedId string, targetHost string, backupFolder string) error {
	cmd := fmt.Sprintf("tar cfz - -C %s . | nc -w 20 %s %d", backupFolder, targetHost, config.Config.SeedPort)
	if runtime.GOOS == "darwin" {
		cmd += " -c"
	}
	err := commandRun(
		fmt.Sprintf(cmd),
		func(cmd *exec.Cmd) {
			activeCommands[seedId] = cmd
			log.Debugf("Sending MySQL backup to %+v", targetHost)
		})
	if err != nil {
		return log.Errore(err)
	}
	return err
}

func StartStreamingBackup(seedId string, targetHost string, databases string) error {
	if databases != "" {
		availiableDatabases, _ := dbagent.GetMySQLDatabases()
		for _, db := range strings.Split(databases, ",") {
			if !contains(db, availiableDatabases) {
				return log.Errorf("Cannot backup database %+v. Database doesn't exists", db)
			}
		}
	}
	err := dbagent.ManageReplicationUser()
	if err != nil {
		log.Errore(err)
	}
	cmd := fmt.Sprintf("innobackupex %s --stream=xbstream --user=%s --password=%s --port=%d --parallel=%d --databases='%s' | nc -w 20 %s %d",
		config.Config.MySQLBackupDir, config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort, config.Config.XtrabackupParallelThreads, strings.Replace(databases, ",", " ", -1), targetHost, config.Config.SeedPort)
	if runtime.GOOS == "darwin" {
		cmd += " -c"
	}
	err = commandRun(
		fmt.Sprintf(cmd),
		func(cmd *exec.Cmd) {
			activeCommands[seedId] = cmd
			log.Debugf("Streaming xtrabackup to %+v", targetHost)
		})
	if err != nil {
		return log.Errore(err)
	}
	return err

}

// well be used in order to perform cleanup in case of restore errors
//func Cleanup() err {
//
//}

func StartRestore(seedId string, seedMethod string, sourceHost string, sourcePort int, backupFolder string, databases string) (err error) {
	if !contains(seedMethod, seedMethods) {
		return log.Errorf("Unsupported seed method")
	}
	if len(databases) > 0 {
		for _, db := range strings.Split(databases, ",") {
			err := config.AddKeyToMySQLConfig("replicate-do-db", db)
			if err != nil {
				return log.Errore(err)
			}
		}
		err := MySQLRestart()
		if err != nil {
			return log.Errore(err)
		}
	}
	// as we are restoring new slave and if we have GTIDs we may have @@GLOBAL.GTID_EXECUTED not empty and we won't be able to set @@GLOBAL.GTID_PURGED
	// so we need to do RESET MASTER on slave
	query := "RESET MASTER;"
	_, err = dbagent.ExecuteQuery(query)
	if err != nil {
		return log.Errore(err)
	}
	if seedMethod == "mysqldump" {
		err = restoreMySQLDump(seedId, backupFolder)
		if err != nil {
			return log.Errore(err)
		}
		err = dbagent.ManageReplicationUser()
		if err != nil {
			return log.Errore(err)
		}
		err = dbagent.StartSlave(sourceHost, sourcePort, "", "", "")
		if err != nil {
			return log.Errore(err)
		}
	}
	if seedMethod == "mydumper" {
		logFile, position, gtidPurged, err := parseMydumperMetadata(backupFolder)
		if err != nil {
			return log.Errore(err)
		}
		//mydumper doesn't set sql_mode correctly, so if we will do the same way as mysqldump does. Remember old sql_mode, than set it to
		//NO_AUTO_VALUE_ON_ZERO and then set it back
		sqlMode, err := dbagent.GetMySQLSql_mode()
		if err != nil {
			return log.Errore(err)
		}
		err = dbagent.SetMySQLSql_mode("NO_AUTO_VALUE_ON_ZERO")
		if err != nil {
			return log.Errore(err)
		}
		err = restoreMydumper(seedId, backupFolder)
		if err != nil {
			return log.Errore(err)
		}
		err = dbagent.ManageReplicationUser()
		if err != nil {
			return log.Errore(err)
		}
		err = dbagent.SetMySQLSql_mode(sqlMode)
		if err != nil {
			return log.Errore(err)
		}
		err = dbagent.StartSlave(sourceHost, sourcePort, logFile, position, gtidPurged)
		if err != nil {
			return log.Errore(err)
		}
	}
	return err
}

func parseMydumperMetadata(backupFolder string) (logFile string, position string, gtidPurged string, err error) {
	file, err := os.Open(path.Join(backupFolder, mydumperMetadataFile))
	if err != nil {
		return "", "", "", log.Errore(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "Log:") {
			logFile = strings.Trim(strings.Split(scanner.Text(), ":")[1], " ")
		}
		if strings.Contains(scanner.Text(), "Pos:") {
			position = strings.Trim(strings.Split(scanner.Text(), ":")[1], " ")
		}
		if strings.Contains(scanner.Text(), "GTID:") {
			gtidPurged = strings.Trim(strings.Split(scanner.Text(), ":")[1], " ")
		}
	}
	return logFile, position, gtidPurged, err
}

func restoreMydumper(seedId string, backupFolder string) error {
	cmd := fmt.Sprintf("myloader -u %s -p %s --port %d -t %d -d %s -o",
		config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort, config.Config.MyDumperParallelThreads, backupFolder)
	err := commandRun(
		fmt.Sprintf(cmd),
		func(cmd *exec.Cmd) {
			activeCommands[seedId] = cmd
			log.Debug("Restoring using mydumper")
		})
	if err != nil {
		return log.Errore(err)
	}
	return err
}

func restoreMySQLDump(seedId string, backupFolder string) error {
	if config.Config.CompressLogicalBackup {
		cmd := fmt.Sprintf("gunzip -c %s > %s", path.Join(backupFolder, mysqlbackupCompressedFileName), path.Join(backupFolder, mysqlbackupFileName))
		err := commandRun(
			fmt.Sprintf(cmd),
			func(cmd *exec.Cmd) {
				activeCommands[seedId] = cmd
				log.Debug("Extracting mysqldump backup")
			})
		if err != nil {
			return log.Errore(err)
		}
	}
	cmd := fmt.Sprintf("mysql -u%s -p%s --port %d < %s", config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort, path.Join(backupFolder, mysqlbackupFileName))
	err := commandRun(
		fmt.Sprintf(cmd),
		func(cmd *exec.Cmd) {
			activeCommands[seedId] = cmd
			log.Debug("Restoring using mysqlbackup")
		})
	if err != nil {
		return log.Errore(err)
	}
	return err
}
