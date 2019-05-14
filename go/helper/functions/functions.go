package functions

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/github/orchestrator-agent/go/config"
	"github.com/openark/golib/log"
	"github.com/openark/golib/sqlutils"
)

// Add sudo to a command if we're configured to do so.  Otherwise just a signifier of a
// privileged command
func SudoCmd(commandText string) string {
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

// CommandOutput executes a command and return output bytes
func CommandOutput(commandText string) ([]byte, error) {
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

// CommandRun executes a command
func CommandRun(commandText string, onCommand func(*exec.Cmd)) error {
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

func OutputLines(commandOutput []byte, err error) ([]string, error) {
	if err != nil {
		return nil, err
	}
	text := strings.Trim(fmt.Sprintf("%s", commandOutput), "\n")
	lines := strings.Split(text, "\n")
	return lines, err
}

func OutputTokens(delimiterPattern string, commandOutput []byte, err error) ([][]string, error) {
	lines, err := OutputLines(commandOutput, err)
	if err != nil {
		return nil, err
	}
	tokens := make([][]string, len(lines))
	for i := range tokens {
		tokens[i] = regexp.MustCompile(delimiterPattern).Split(lines[i], -1)
	}
	return tokens, err
}

func OpenConnection(user string, password string, port int) (*sql.DB, error) {
	mysqlURI := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?interpolateParams=true&timeout=1s",
		user,
		password,
		"localhost",
		port,
		"mysql",
	)
	db, _, err := sqlutils.GetDB(mysqlURI)
	db.SetMaxIdleConns(0)
	err = db.Ping()
	return db, err
}

func QueryData(user string, password string, port int, query string, argsArray []interface{}, onRow func(sqlutils.RowMap) error) error {
	db, err := OpenConnection(user, password, port)
	if err != nil {
		return err
	}
	return sqlutils.QueryRowsMap(db, query, onRow, argsArray...)
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
	return err
}

// DeleteFile deletes file located in folder. Can be used with wildcards
func DeleteFile(path string, file string) error {
	files, err := filepath.Glob(filepath.Join(path, file))
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil {
			return err
		}
	}
	return err
}

//ChownRecurse changes recursive owner for all files and folders in path
func ChownRecurse(path string, uid int, gid int) error {
	return filepath.Walk(path, func(name string, info os.FileInfo, err error) error {
		if err == nil {
			err = os.Chown(name, uid, gid)
		}
		return err
	})
}

func GetLogicalVolumeFSType(volumeName string) (string, error) {
	command := fmt.Sprintf("blkid %s", volumeName)
	output, err := CommandOutput(SudoCmd(command))
	lines, err := OutputLines(output, err)
	re := regexp.MustCompile(`TYPE="(.*?)"`)
	for _, line := range lines {
		fsType := re.FindStringSubmatch(line)[1]
		return fsType, nil
	}
	return "", fmt.Errorf("Cannot find FS type for logical volume %s", volumeName)
}

func MountLV(mountPoint string, volumeName string) error {
	if volumeName == "" {
		return fmt.Errorf("Empty columeName in MountLV")
	}
	fsType, err := GetLogicalVolumeFSType(volumeName)
	if err != nil {
		return err
	}

	mountOptions := ""
	if fsType == "xfs" {
		mountOptions = "-o nouuid"
	}
	_, err = CommandOutput(SudoCmd(fmt.Sprintf("mount %s %s %s", mountOptions, volumeName, mountPoint)))
	return err
}

func RemoveLV(volumeName string) error {
	_, err := CommandOutput(SudoCmd(fmt.Sprintf("lvremove --force %s", volumeName)))
	return err
}

func Unmount(mountPoint string) error {
	_, err := CommandOutput(SudoCmd(fmt.Sprintf("umount %s", mountPoint)))
	return err
}

func CreateSnapshot(snapshotSize string, snapshotName string, snapshotVolumeGroup string, snapshotLogicalVolume string) error {
	_, err := CommandOutput(fmt.Sprintf("lvcreate --size %s --snapshot --name %s /dev/%s/%s", snapshotSize, snapshotName, snapshotVolumeGroup, snapshotLogicalVolume))
	return err
}

func MySQLStop() error {
	_, err := CommandOutput(config.Config.MySQLServiceStopCommand)
	return err
}

func MySQLStart() error {
	_, err := CommandOutput(config.Config.MySQLServiceStartCommand)
	return err
}
