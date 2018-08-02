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
var mysqlUsersTables = []string{"user", "columns_priv", "procs_priv", "proxies_priv", "tables_priv"}
var systemDatabases = []string{"mysql", "information_schema"}

const (
	mysqlbackupFileName           = "backup.sql"
	mysqlbackupCompressedFileName = "backup.sql.gz"
	mysqlUserBackupFileName       = "mysql_users_backup.sql"
	mydumperMetadataFile          = "metadata"
	xtrabackupMetadataFile        = "xtrabackup_binlog_info"
	mysqlRestartSleepInterval     = 40
	mysqlBackupDatadirName        = "mysql_datadir_backup.tar.gz"
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

type BinlogType int

const (
	BinaryLog BinlogType = iota
	RelayLog
)

// BinlogCoordinates described binary log coordinates in the form of log file & log position.
type BinlogCoordinates struct {
	LogFile string
	LogPos  int64
	Type    BinlogType
}

type BackupMetadata struct {
	BinlogCoordinates BinlogCoordinates
	GTIDPurged        string
}

// GetRelayLogIndexFileName attempts to find the relay log index file under the mysql datadir
func GetRelayLogIndexFileName() (string, error) {
	config.Config.RLock()
	defer config.Config.RUnlock()
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

func mySQLBinlogContentHeaderSize(binlogFile string) (int64, error) {
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
		if headerSize, err = mySQLBinlogContentHeaderSize(binlogFiles[0]); err != nil {
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
			if headerSize, err = mySQLBinlogContentHeaderSize(binlogFile); err != nil {
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
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
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

// commandStart executes a command and doesn't wait for it to complete
func commandStart(commandText string, onCommand func(*exec.Cmd)) error {
	var stderr bytes.Buffer
	cmd, tmpFileName, err := execCmd(commandText)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stderr = &stderr
	if err != nil {
		return log.Errore(err)
	}

	onCommand(cmd)

	err = cmd.Start()
	if err != nil {
		return log.Errorf(stderr.String())
	}
	time.Sleep(1000 * time.Millisecond)
	defer os.Remove(tmpFileName)

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

func getLogicalVolumePath(volumeName string) (string, error) {
	if logicalVolumes, err := LogicalVolumes(volumeName, ""); err == nil && len(logicalVolumes) > 0 {
		return logicalVolumes[0].Path, err
	}
	return "", errors.New(fmt.Sprintf("logical volume not found: %+v", volumeName))
}

func getLogicalVolumeFSType(volumeName string) (string, error) {
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
		mount.LVPath, _ = getLogicalVolumePath(mount.Device)
		mount.DiskUsage, _ = diskUsed(mountPoint)
		mount.MySQLDataPath, _ = heuristicMySQLDataPath(mountPoint)
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
	fsType, err := getLogicalVolumeFSType(volumeName)
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

func getDiskSpace(path string) (DiskStats, error) {
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

func diskUsed(path string) (int64, error) {
	du, err := getDiskSpace(path)
	return du.Used, err
}

func DiskFree(path string) (int64, error) {
	du, err := getDiskSpace(path)
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

// deleteFile deletes file located in folder. Can be used with wildcards
func deleteFile(path string, file string) error {
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

func heuristicMySQLDataPath(mountPoint string) (string, error) {
	config.Config.RLock()
	defer config.Config.RUnlock()
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

func MySQLErrorLogTail(errorLog string) ([]string, error) {
	lines := 20
	var bufsize int64 = 512
	var result []string
	var iter int64

	file, err := os.Open(errorLog)
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
		if currentPosition > 0 {
			data = data[1:]
		}
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
	cmd := fmt.Sprintf("%s; sleep %d", config.Config.MySQLServiceStartCommand, mysqlRestartSleepInterval)
	_, err := commandOutput(cmd)
	return err
}

func mySQLRestart() error {
	cmd := fmt.Sprintf("%s; sleep %d", config.Config.MySQLServiceRestartCommand, mysqlRestartSleepInterval)
	_, err := commandOutput(cmd)
	return err
}

func ReceiveMySQLSeedData(seedId string) error {
	config.Config.RLock()
	defer config.Config.RUnlock()
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
	config.Config.RLock()
	defer config.Config.RUnlock()
	dbi := make(map[string]*MySQLDatabase)
	var physicalSize, tokuPhysicalSize, logicalSize int64 = 0, 0, 0
	databases, err := dbagent.GetMySQLDatabases()
	if err != nil {
		return dbinfo, err
	}
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
	return dbinfo, log.Errore(err)
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

func CreateBackupFolder(seedId string, backupFolderPath string) (backupFolder string, err error) {
	if backupFolderPath == "" {
		backupFolder = path.Join(config.Config.MySQLBackupDir, time.Now().Format("20060102-150405"))
	} else {
		backupFolder = backupFolderPath
	}
	err = os.Mkdir(backupFolder, 0755)
	return backupFolder, log.Errore(err)
}

func StartBackup(seedId string, seedMethod string, backupFolder string, databases []string, targetHost string) (err error) {
	var cmd string
	if !contains(seedMethod, seedMethods) {
		log.Errorf("Unsupported seed method")
	}
	if seedMethod == "xtrabackup-stream" && targetHost == "" {
		log.Errorf("Target host should be specified when using xtrabackup-stream")
	}
	// if we optionally pass some databases, let's check that they exists
	if len(databases) != 0 {
		availiableDatabases, _ := dbagent.GetMySQLDatabases()
		for _, db := range databases {
			if !contains(db, availiableDatabases) {
				log.Errorf("Cannot backup database %+v. Database doesn't exists", db)
			}
		}
		//if we don't already have mysql database in databases, add it
		if !contains("mysql", databases) {
			databases = append(databases, "mysql")
		}
		//and if we are on 5.7 we need to add sys db because sometimes during mysql_upgrade run we can get errors like mysql_upgrade: [ERROR] 1813: Tablespace '`sys`.`sys_config`' exists
		version, _ := dbagent.GetMySQLVersion()
		if version == "5.7" && !contains("sys", databases) {
			databases = append(databases, "sys")
		}
	}
	// create command for running backup using defined seed method
	switch seedMethod {
	case "xtrabackup":
		cmd = backupXtrabackupCmd(backupFolder, databases)
	case "mydumper":
		cmd = backupMydumperCmd(backupFolder, databases)
	case "mysqldump":
		cmd = backupMysqldumpCmd(backupFolder, databases)
	case "xtrabackup-stream":
		cmd = backupXtrabackupStreamCmd(targetHost, databases)
	}
	err = commandRun(
		cmd,
		func(cmd *exec.Cmd) {
			activeCommands[seedId] = cmd
			log.Debugf("Start backup using %+v seed method", seedMethod)
		})
	return log.Errore(err)
}

func backupXtrabackupStreamCmd(targetHost string, databases []string) (cmd string) {
	config.Config.RLock()
	defer config.Config.RUnlock()
	cmd = fmt.Sprintf("innobackupex %s --stream=xbstream --user=%s --password=%s --port=%d --parallel=%d --databases='%s' | nc -w 20 %s %d",
		config.Config.MySQLBackupDir, config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort, config.Config.XtrabackupParallelThreads, strings.Join(databases, " "), targetHost, config.Config.SeedPort)
	if runtime.GOOS == "darwin" {
		cmd += " -c"
	}
	return cmd
}

func backupMysqldumpCmd(backupFolder string, databases []string) (cmd string) {
	config.Config.RLock()
	defer config.Config.RUnlock()
	// we need to comment out SET @@GLOBAL.GTID_PURGED to be able to restore dump. Later we will issue RESET MASTER and SET @@GLOBAL.GTID_PURGED from orchestrator side
	if len(databases) == 0 {
		cmd = fmt.Sprintf("mysqldump --user=%s --password=%s --port=%d --single-transaction --default-character-set=utf8mb4 --master-data=2 --routines --events --triggers --all-databases | sed -e 's/SET @@GLOBAL.GTID_PURGED=/-- SET @@GLOBAL.GTID_PURGED=/g' ",
			config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort)
	} else {
		cmd = fmt.Sprintf("mysqldump --user=%s --password=%s --port=%d --single-transaction --default-character-set=utf8mb4 --master-data=2 --routines --events --triggers --databases %s | sed -e 's/SET @@GLOBAL.GTID_PURGED=/-- SET @@GLOBAL.GTID_PURGED=/g' ",
			config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort, strings.Join(databases, " "))
	}
	if config.Config.CompressLogicalBackup {
		cmd += fmt.Sprintf(" | gzip > %s", path.Join(backupFolder, mysqlbackupCompressedFileName))
	} else {
		cmd += fmt.Sprintf(" > %s", path.Join(backupFolder, mysqlbackupFileName))
	}
	return cmd
}

func backupMydumperCmd(backupFolder string, databases []string) (cmd string) {
	config.Config.RLock()
	defer config.Config.RUnlock()
	cmd = fmt.Sprintf("mydumper --user=%s --password=%s --port=%d --threads=%d --outputdir=%s --triggers --events --routines --regex='(%s)'",
		config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort, config.Config.MyDumperParallelThreads, backupFolder, strings.Join(databases, "\\.|")+"\\.")
	if config.Config.CompressLogicalBackup {
		cmd += fmt.Sprintf(" --compress")
	}
	if config.Config.MyDumperRowsChunkSize != 0 {
		cmd += fmt.Sprintf(" --rows=%d", config.Config.MyDumperRowsChunkSize)
	}
	return cmd
}

func backupXtrabackupCmd(backupFolder string, databases []string) string {
	config.Config.RLock()
	defer config.Config.RUnlock()
	return fmt.Sprintf("xtrabackup --backup --user=%s --password=%s --port=%d --parallel=%d --target-dir=%s --databases='%s'",
		config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort, config.Config.XtrabackupParallelThreads, backupFolder, strings.Join(databases, " "))
}

func backupMySQLUsers(seedId string) (err error) {
	config.Config.RLock()
	defer config.Config.RUnlock()
	if len(config.Config.MySQLBackupUsersOnTargetHost) > 0 {
		statement, err := dbagent.GenerateBackupForUsers(config.Config.MySQLBackupUsersOnTargetHost)
		if err != nil {
			return log.Errore(err)
		}
		f, err := os.OpenFile(path.Join(config.Config.MySQLBackupDir, mysqlUserBackupFileName), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
		defer f.Close()
		if err != nil {
			return log.Errore(err)
		}
		_, err = f.WriteString(statement)
		if err != nil {
			return log.Errore(err)
		}
	} else {
		cmd := fmt.Sprintf("mysqldump --master-data=2 --set-gtid-purged=OFF --user=%s --password=%s --port=%d --single-transaction mysql %s > %s",
			config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort, strings.Join(mysqlUsersTables, " "), path.Join(config.Config.MySQLBackupDir, mysqlUserBackupFileName))
		err = commandRun(
			cmd,
			func(cmd *exec.Cmd) {
				activeCommands[seedId] = cmd
				log.Debug("Backing up MySQL users")
			})
	}
	cmd := fmt.Sprintf("echo 'FLUSH PRIVILEGES;' >> %s", path.Join(config.Config.MySQLBackupDir, mysqlUserBackupFileName))
	err = commandRun(
		cmd,
		func(cmd *exec.Cmd) {
			activeCommands[seedId] = cmd
			log.Debug("Adding FLUSH PRIVILEGES command to MySQL users backup")
		})
	return log.Errore(err)
}

func restoreMySQLUsers(seedId string, backupFolder string) error {
	config.Config.RLock()
	defer config.Config.RUnlock()
	cmd := fmt.Sprintf("mysql -u%s -p%s --port %d mysql < %s", config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort, path.Join(config.Config.MySQLBackupDir, mysqlUserBackupFileName))
	err := commandRun(
		cmd,
		func(cmd *exec.Cmd) {
			activeCommands[seedId] = cmd
			log.Debug("Restoring MySQL users")
		})
	return log.Errore(err)
}

func ReceiveBackup(seedId string, seedMethod string, backupFolder string) error {
	config.Config.RLock()
	defer config.Config.RUnlock()
	var cmd string
	var err error
	var MySQLInnoDBLogDir string
	if !contains(seedMethod, seedMethods) {
		return log.Errorf("Unsupported seed method")
	}
	if config.Config.MySQLBackupOldDatadir {
		if err := MySQLStop(); err != nil {
			return log.Errore(err)
		}
		cmd := fmt.Sprintf("tar zcfp %s -C %s .", path.Join(config.Config.MySQLBackupDir, mysqlBackupDatadirName), config.Config.MySQLDataDir)
		err := commandRun(
			cmd,
			func(cmd *exec.Cmd) {
				activeCommands[seedId] = cmd
				log.Debugf("Backing up old datadir")
			})
		if err != nil {
			return log.Errore(err)
		}
		if err := MySQLStart(); err != nil {
			return log.Errore(err)
		}
	}
	if backupFolder == config.Config.MySQLDataDir && seedMethod == "xtrabackup-stream" {
		if len(config.Config.MySQLInnoDBLogDir) == 0 {
			MySQLInnoDBLogDir = config.Config.MySQLDataDir
		} else {
			MySQLInnoDBLogDir = config.Config.MySQLInnoDBLogDir
		}
		if err := backupMySQLUsers(seedId); err != nil {
			return log.Errore(err)
		}
		if err := MySQLStop(); err != nil {
			return log.Errore(err)
		}
		if err := deleteFile(MySQLInnoDBLogDir, "ib_logfile*"); err != nil {
			return log.Errore(err)
		}
		if err := DeleteDirContents(config.Config.MySQLDataDir); err != nil {
			return log.Errore(err)
		}
	}
	cmd = fmt.Sprintf("sleep 1; nc -l -p %d ", config.Config.SeedPort)
	switch seedMethod {
	case "xtrabackup-stream":
		cmd += fmt.Sprintf("| xbstream -x -C %s", backupFolder)
	default:
		cmd += fmt.Sprintf("| tar xfz - -C %s", backupFolder)
	}
	err = commandStart(
		cmd,
		func(cmd *exec.Cmd) {
			activeCommands[seedId] = cmd
			log.Debug("Receiving MySQL backup")
		})
	return log.Errore(err)
}

func SendBackup(seedId string, targetHost string, backupFolder string) error {
	cmd := fmt.Sprintf("tar cfz - -C %s . | nc  %s %d", backupFolder, targetHost, config.Config.SeedPort)
	if runtime.GOOS == "darwin" {
		cmd += " -c"
	}
	err := commandRun(
		cmd,
		func(cmd *exec.Cmd) {
			activeCommands[seedId] = cmd
			log.Debugf("Sending MySQL backup to %+v", targetHost)
		})
	return log.Errore(err)
}

func CleanupMySQLBackupDir(seedId string) error {
	err := DeleteDirContents(config.Config.MySQLBackupDir)
	// if we have some active commands, let's kill them to prevent future errors
	cmd := activeCommands[seedId]
	if cmd != nil {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	return log.Errore(err)
}

func StartRestore(seedId string, seedMethod string, backupFolder string, databases []string) (err error) {
	config.Config.RLock()
	defer config.Config.RUnlock()
	err = startRestore(seedId, seedMethod, backupFolder, databases)
	// if we backed up old datadir and have errors during restore process, let's remove contents of datadir and move back old datadir
	if err != nil && config.Config.MySQLBackupOldDatadir {
		// stop MySQL
		if err := MySQLStop(); err != nil {
			return log.Errore(err)
		}
		if err := DeleteDirContents(config.Config.MySQLDataDir); err != nil {
			return log.Errore(err)
		}
		cmd := fmt.Sprintf("tar zxfp %s -C %s", path.Join(config.Config.MySQLBackupDir, mysqlBackupDatadirName), config.Config.MySQLDataDir)
		err := commandRun(
			cmd,
			func(cmd *exec.Cmd) {
				activeCommands[seedId] = cmd
				log.Debugf("Restoring old datadir")
			})
		if err != nil {
			return log.Errore(err)
		}
		if err := MySQLStart(); err != nil {
			return log.Errore(err)
		}
	}
	return log.Errore(err)
}

func startRestore(seedId string, seedMethod string, backupFolder string, databases []string) (err error) {
	config.Config.RLock()
	defer config.Config.RUnlock()
	if !contains(seedMethod, seedMethods) {
		return log.Errorf("Unsupported seed method")
	}
	if len(databases) > 0 {
		for _, db := range databases {
			if err := config.AddKeyToMySQLConfig("replicate-do-db", db); err != nil {
				return log.Errore(err)
			}
		}
		// we do not need to restart MySQL in case of xtrabackup-stream to datadir as it's already stopped
		if seedMethod != "xtrabackup-stream" && backupFolder != config.Config.MySQLDataDir {
			if err := mySQLRestart(); err != nil {
				return log.Errore(err)
			}
		}
	}
	// Backup users if we are not streaming to datadir. In case of streaming to datadir users were backuped when we recieved backup
	if backupFolder != config.Config.MySQLDataDir {
		if err := backupMySQLUsers(seedId); err != nil {
			return log.Errore(err)
		}
	}
	switch seedMethod {
	case "xtrabackup", "xtrabackup-stream":
		if err := restoreXtrabackup(seedId, backupFolder, databases); err != nil {
			return log.Errore(err)
		}
	case "mydumper":
		if err := restoreMydumper(seedId, backupFolder); err != nil {
			return log.Errore(err)
		}
	case "mysqldump":
		if err := restoreMySQLDump(seedId, backupFolder); err != nil {
			return log.Errore(err)
		}
	}
	// just execute CHANGE MASTER TO in order to save replication user and password. All other will be done by orchestrator
	if err := dbagent.SetReplicationUserAndPassword(); err != nil {
		return log.Errore(err)
	}
	// restore users
	if err := restoreMySQLUsers(seedId, backupFolder); err != nil {
		return log.Errore(err)
	}
	return err
}

func restoreXtrabackup(seedId string, backupFolder string, databases []string) (err error) {
	var MySQLInnoDBLogDir string
	if len(config.Config.MySQLInnoDBLogDir) == 0 {
		MySQLInnoDBLogDir = config.Config.MySQLDataDir
	} else {
		MySQLInnoDBLogDir = config.Config.MySQLInnoDBLogDir
	}
	if err := prepareXtrabackup(seedId, backupFolder); err != nil {
		return log.Errore(err)
	}
	// xtrabackup full\partial, xtrabackup-stream full\partial to MySQLBackupDir
	if backupFolder != config.Config.MySQLDataDir {
		if err := MySQLStop(); err != nil {
			return log.Errore(err)
		}
		if err := deleteFile(MySQLInnoDBLogDir, "ib_logfile*"); err != nil {
			return log.Errore(err)
		}
		if err := DeleteDirContents(config.Config.MySQLDataDir); err != nil {
			return log.Errore(err)
		}
		if err := copyXtrabackup(seedId, backupFolder); err != nil {
			return log.Errore(err)
		}
		if err := deleteFile(MySQLInnoDBLogDir, "ib_logfile*"); err != nil {
			return log.Errore(err)
		}
	}
	if err := changeDatadirPermissions(seedId); err != nil {
		return log.Errore(err)
	}
	if err := MySQLStart(); err != nil {
		return log.Errore(err)
	}
	if len(databases) > 0 {
		if err := runMySQLUpgrade(seedId); err != nil {
			return log.Errore(err)
		}
		if err := mySQLRestart(); err != nil {
			return log.Errore(err)
		}
	}
	return err
}

func restoreMydumper(seedId string, backupFolder string) error {
	//mydumper doesn't set sql_mode correctly, so we will do the same way as mysqldump does. Remember old sql_mode, than set it to
	//NO_AUTO_VALUE_ON_ZERO and then set it back
	sqlMode, err := dbagent.GetMySQLSql_mode()
	if err != nil {
		return log.Errore(err)
	}
	if err := dbagent.SetMySQLSql_mode("NO_AUTO_VALUE_ON_ZERO"); err != nil {
		return log.Errore(err)
	}
	if err := restoreMydumperCmd(seedId, backupFolder); err != nil {
		return log.Errore(err)
	}
	if err := dbagent.SetMySQLSql_mode(sqlMode); err != nil {
		return log.Errore(err)
	}
	return err
}

func restoreMydumperCmd(seedId string, backupFolder string) error {
	config.Config.RLock()
	defer config.Config.RUnlock()
	cmd := fmt.Sprintf("myloader -u %s -p %s -o --port %d -t %d -d %s",
		config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort, config.Config.MyDumperParallelThreads, backupFolder)
	err := commandRun(
		cmd,
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
	config.Config.RLock()
	defer config.Config.RUnlock()
	if config.Config.CompressLogicalBackup {
		cmd := fmt.Sprintf("gunzip -c %s > %s", path.Join(backupFolder, mysqlbackupCompressedFileName), path.Join(backupFolder, mysqlbackupFileName))
		err := commandRun(
			cmd,
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
		cmd,
		func(cmd *exec.Cmd) {
			activeCommands[seedId] = cmd
			log.Debug("Restoring using mysqlbackup")
		})
	if err != nil {
		return log.Errore(err)
	}
	return err
}

func copyXtrabackup(seedId string, backupFolder string) error {
	cmd := fmt.Sprintf("xtrabackup --copy-back --target-dir=%s", backupFolder)
	err := commandRun(
		cmd,
		func(cmd *exec.Cmd) {
			activeCommands[seedId] = cmd
			log.Debug("Copying xtrabackup to datadir")
		})
	return err
}

func prepareXtrabackup(seedId string, backupFolder string) error {
	cmd := fmt.Sprintf("xtrabackup --prepare --target-dir=%s", backupFolder)
	err := commandRun(
		cmd,
		func(cmd *exec.Cmd) {
			activeCommands[seedId] = cmd
			log.Debug("Preparing xtrabackup")
		})
	return err
}

func changeDatadirPermissions(seedId string) error {
	config.Config.RLock()
	defer config.Config.RUnlock()
	cmd := fmt.Sprintf("chown -R mysql:mysql %s", config.Config.MySQLDataDir)
	err := commandRun(
		cmd,
		func(cmd *exec.Cmd) {
			activeCommands[seedId] = cmd
			log.Debug("Changing permissions on MySQL datadir")
		})
	return err
}

func runMySQLUpgrade(seedId string) error {
	config.Config.RLock()
	defer config.Config.RUnlock()
	cmd := fmt.Sprintf("mysql_upgrade --protocol=tcp -u%s -p%s --port %d --force", config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort)
	err := commandRun(
		cmd,
		func(cmd *exec.Cmd) {
			activeCommands[seedId] = cmd
			log.Debug("Running mysql_upgrade")
		})
	return err
}

func GetBackupMetadata(seedId string, seedMethod string, backupFolder string) (BackupMetadata, error) {
	var logFile, gtidPurged = "", ""
	var err error
	var position int64
	switch seedMethod {
	case "xtrabackup", "xtrabackup-stream":
		logFile, position, gtidPurged, err = parseXtrabackupMetadata(backupFolder)
	case "mydumper":
		logFile, position, gtidPurged, err = parseMydumperMetadata(backupFolder)
	case "mysqldump":
		logFile, position, gtidPurged, err = parseMysqldumpMetadata(backupFolder)
	}
	meta := BackupMetadata{
		BinlogCoordinates: BinlogCoordinates{
			LogFile: logFile,
			LogPos:  position,
		},
		GTIDPurged: gtidPurged,
	}
	return meta, log.Errore(err)
}

func parseXtrabackupMetadata(backupFolder string) (logFile string, position int64, gtidPurged string, err error) {
	var params []string
	file, err := os.Open(path.Join(backupFolder, xtrabackupMetadataFile))
	if err != nil {
		return "", 0, "", log.Errore(err)
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	metadata, err := reader.ReadString('\n')
	if err != nil {
		return "", 0, "", log.Errore(err)
	}
	params = strings.Split(metadata, "\t")
	logFile = params[0]
	position, err = strconv.ParseInt(strings.Trim(params[1], "\n"), 10, 64)
	if err != nil {
		return "", 0, "", log.Errore(err)
	}
	if len(params) > 2 {
		gtidPurged = strings.Trim(params[2], "\n")
	}
	return logFile, position, gtidPurged, err
}

func parseMysqldumpMetadata(backupFolder string) (logFile string, position int64, gtidPurged string, err error) {
	file, err := os.Open(path.Join(backupFolder, mysqlbackupFileName))
	if err != nil {
		return "", 0, "", log.Errore(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "GTID_PURGED") {
			gtidPurged = strings.Replace(strings.Replace(strings.Split(scanner.Text(), "=")[1], "'", "", -1), ";", "", -1)
		}
		if strings.Contains(scanner.Text(), "CHANGE MASTER") {
			logFile = strings.Replace(strings.Split(strings.Split(scanner.Text(), ",")[0], "=")[1], "'", "", -1)
			position, err = strconv.ParseInt(strings.Replace(strings.Split(strings.Split(scanner.Text(), ",")[1], "=")[1], ";", "", -1), 10, 64)
			if err != nil {
				return "", 0, "", log.Errore(err)
			}
			break
		}
	}
	return logFile, position, gtidPurged, err
}

func parseMydumperMetadata(backupFolder string) (logFile string, position int64, gtidPurged string, err error) {
	file, err := os.Open(path.Join(backupFolder, mydumperMetadataFile))
	if err != nil {
		return "", 0, "", log.Errore(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "Log:") {
			logFile = strings.Trim(strings.Split(scanner.Text(), ":")[1], " ")
		}
		if strings.Contains(scanner.Text(), "Pos:") {
			position, err = strconv.ParseInt(strings.Trim(strings.Split(scanner.Text(), ":")[1], " "), 10, 64)
			if err != nil {
				return "", 0, "", log.Errore(err)
			}
		}

		if strings.Contains(scanner.Text(), "GTID:") {
			gtidPurged = strings.Trim(strings.SplitAfterN(scanner.Text(), ":", 2)[1], " ")
			break
		}
	}
	return logFile, position, gtidPurged, err
}
