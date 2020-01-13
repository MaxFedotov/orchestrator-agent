package osagent

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/outbrain/golib/log"
)

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
