package osagent

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/github/orchestrator-agent/go/helper/cmd"
)

func MySQLBinlogContents(binlogFiles []string, startPosition int64, stopPosition int64, execWithSudo bool) (string, error) {
	if len(binlogFiles) == 0 {
		return "", fmt.Errorf("No binlog files provided in MySQLBinlogContents")
	}
	command := `mysqlbinlog`
	for _, binlogFile := range binlogFiles {
		command = fmt.Sprintf("%s %s", command, binlogFile)
	}
	if startPosition != 0 {
		command = fmt.Sprintf("%s --start-position=%d", command, startPosition)
	}
	if stopPosition != 0 {
		command = fmt.Sprintf("%s --stop-position=%d", command, stopPosition)
	}
	command = fmt.Sprintf("%s | gzip | base64", command)

	output, err := cmd.CommandOutput(command, execWithSudo)
	return string(output), err
}

func MySQLBinlogContentHeaderSize(binlogFile string, execWithSudo bool) (int64, error) {
	// magic header
	// There are the first 4 bytes, and then there's also the first entry (the format-description).
	// We need both from the first log file.
	// Typically, the format description ends at pos 120, but let's verify...

	command := fmt.Sprintf("mysqlbinlog %s --start-position=4 | head | egrep -o 'end_log_pos [^ ]+' | head -1 | awk '{print $2}'", binlogFile)
	if content, err := cmd.CommandOutput(command, execWithSudo); err != nil {
		return 0, err
	} else {
		return strconv.ParseInt(strings.TrimSpace(string(content)), 10, 0)
	}
}

func MySQLBinlogBinaryContents(binlogFiles []string, startPosition int64, stopPosition int64, execWithSudo bool) (result string, err error) {
	if len(binlogFiles) == 0 {
		return "", fmt.Errorf("No binlog files provided in MySQLBinlogContents")
	}
	tmpFile, err := ioutil.TempFile("", "orchestrator-agent-binlog-contents-")
	if err != nil {
		return "", err
	}
	var headerSize int64
	if startPosition != 0 {
		if headerSize, err = MySQLBinlogContentHeaderSize(binlogFiles[0], execWithSudo); err != nil {
			return "", err
		}
		command := fmt.Sprintf("cat %s | head -c%d >> %s", binlogFiles[0], headerSize, tmpFile.Name())
		if _, err := cmd.CommandOutput(command, execWithSudo); err != nil {
			return "", err
		}
	}
	for i, binlogFile := range binlogFiles {
		command := fmt.Sprintf("cat %s", binlogFile)

		if i == len(binlogFiles)-1 && stopPosition != 0 {
			command = fmt.Sprintf("%s | head -c %d", command, stopPosition)
		}
		if i == 0 && startPosition != 0 {
			command = fmt.Sprintf("%s | tail -c+%d", command, startPosition+1)
		}
		if i > 0 {
			// At any case, we drop out binlog header (magic + format_description) for next relay logs
			if headerSize, err = MySQLBinlogContentHeaderSize(binlogFile, execWithSudo); err != nil {
				return "", err
			}
			command = fmt.Sprintf("%s | tail -c+%d", command, headerSize+1)
		}
		command = fmt.Sprintf("%s >> %s", command, tmpFile.Name())
		if err := cmd.CommandRun(command, execWithSudo); err != nil {
			return "", err
		}
	}

	command := fmt.Sprintf("cat %s | gzip | base64", tmpFile.Name())
	output, err := cmd.CommandOutput(command, execWithSudo)
	return string(output), err
}
