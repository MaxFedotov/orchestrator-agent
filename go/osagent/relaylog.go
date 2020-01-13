package osagent

import (
	"fmt"
	"io/ioutil"
	"path"
	"strconv"
	"strings"

	"github.com/github/orchestrator-agent/go/config"
	"github.com/github/orchestrator-agent/go/inst"
	"github.com/outbrain/golib/log"
)

// GetRelayLogIndexFileName attempts to find the relay log index file under the mysql datadir
func GetRelayLogIndexFileName() (string, error) {
	directory, err := GetMySQLDataDir()
	if err != nil {
		return "", log.Errore(err)
	}

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
