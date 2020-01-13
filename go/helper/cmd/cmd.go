package cmd

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	log "github.com/sirupsen/logrus"
)

var logger = log.WithFields(log.Fields{"prefix": "CMD"})

func commandSplit(commandText string) (string, []string) {
	tokens := regexp.MustCompile(`[ ]+`).Split(strings.TrimSpace(commandText), -1)
	return tokens[0], tokens[1:]
}

func execCmd(commandText string, execWithSudo bool) *exec.Cmd {
	if execWithSudo {
		commandText = "sudo " + commandText
	}
	logger.WithFields(log.Fields{"cmd": commandText}).Debug("Command executed")
	command, args := commandSplit(commandText)
	return exec.Command(command, args...)
}

// CommandOutput executes a command and return output bytes
func CommandOutput(commandText string, execWithSudo bool) ([]byte, error) {
	cmd := execCmd(commandText, execWithSudo)
	outputBytes, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return outputBytes, nil
}

// CommandRun executes a command
func CommandRun(commandText string, execWithSudo bool) error {
	cmd := execCmd(commandText, execWithSudo)
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

// CommandRunFunc executes a command and runs specified function
func CommandRunFunc(commandText string, execWithSudo bool, onCommand func(*exec.Cmd)) error {
	cmd := execCmd(commandText, execWithSudo)
	onCommand(cmd)
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func OutputLines(commandOutput []byte) []string {
	text := strings.Trim(fmt.Sprintf("%s", commandOutput), "\n")
	lines := strings.Split(text, "\n")
	return lines
}

func OutputTokens(delimiterPattern string, commandOutput []byte) [][]string {
	lines := OutputLines(commandOutput)
	tokens := make([][]string, len(lines))
	for i := range tokens {
		tokens[i] = regexp.MustCompile(delimiterPattern).Split(lines[i], -1)
	}
	return tokens
}
