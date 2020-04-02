package osagent

import (
	"fmt"
	"io/ioutil"
	"path"
	"strconv"
	"strings"

	"github.com/github/orchestrator-agent/go/helper/cmd"
	"github.com/github/orchestrator-agent/go/inst"
)

// GetRelayLogIndexFileName attempts to find the relay log index file under the mysql datadir
func GetRelayLogIndexFileName(mysqlDataDir string, cmd *cmd.CmdOpts) (string, error) {
	output, err := cmd.CommandOutput(fmt.Sprintf("ls %s/*relay*.index", mysqlDataDir))
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(fmt.Sprintf("%s", output)), err
}

// GetRelayLogFileNames attempts to find the active relay logs
func GetRelayLogFileNames(mysqlDataDir string, cmd *cmd.CmdOpts) (fileNames []string, err error) {
	relayLogIndexFile, err := GetRelayLogIndexFileName(mysqlDataDir, cmd)
	if err != nil {
		return fileNames, err
	}

	contents, err := ioutil.ReadFile(relayLogIndexFile)
	if err != nil {
		return fileNames, err
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
func GetRelayLogEndCoordinates(mysqlDataDir string, cmd *cmd.CmdOpts) (coordinates *inst.BinlogCoordinates, err error) {
	relaylogFileNames, err := GetRelayLogFileNames(mysqlDataDir, cmd)
	if err != nil {
		return coordinates, err
	}

	lastRelayLogFile := relaylogFileNames[len(relaylogFileNames)-1]
	output, err := cmd.CommandOutput(fmt.Sprintf("du -b %s", lastRelayLogFile))
	tokens := cmd.OutputTokens(`[ \t]+`, output)
	var fileSize int64
	for _, lineTokens := range tokens {
		fileSize, err = strconv.ParseInt(lineTokens[0], 10, 0)
	}
	if err != nil {
		return coordinates, err
	}
	return &inst.BinlogCoordinates{LogFile: lastRelayLogFile, LogPos: fileSize, Type: inst.RelayLog}, nil
}

func ApplyRelaylogContents(content []byte, cmd *cmd.CmdOpts, mysqlUser string, mysqlPassword string) error {
	encodedContentsFile, err := ioutil.TempFile("", "orchestrator-agent-apply-relaylog-encoded-")
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(encodedContentsFile.Name(), content, 0644); err != nil {
		return err
	}

	relaylogContentsFile, err := ioutil.TempFile("", "orchestrator-agent-apply-relaylog-bin-")
	if err != nil {
		return err
	}

	command := fmt.Sprintf("cat %s | base64 --decode | gunzip > %s", encodedContentsFile.Name(), relaylogContentsFile.Name())
	if err := cmd.CommandRun(command); err != nil {
		return err
	}

	command = fmt.Sprintf("mysqlbinlog %s | mysql --user %s --password %s", relaylogContentsFile.Name(), mysqlUser, mysqlPassword)
	return err
}
