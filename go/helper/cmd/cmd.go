package cmd

import (
	"fmt"
	"regexp"
	"strings"

	log "github.com/sirupsen/logrus"
	"gopkg.in/pipe.v2"
)

var logger = log.WithFields(log.Fields{"prefix": "CMD"})

func commandSplit(commandText string) (string, []string) {
	var args []string
	cmd := ""
	re := regexp.MustCompile(`[-./.\d\w]\S*\d?|"(?:\\"|[^"])+"|'(?:\\"|[^"])+'`)
	res := re.FindAllStringSubmatch(commandText, -1)

	for idx, match := range res {
		for _, arg := range match {
			if idx == 0 {
				cmd = arg
			} else {
				if arg == "' '" || arg == "\" \"" {
					args = append(args, " ")
				} else {
					args = append(args, arg)
				}
			}
		}
	}
	return cmd, args

	// re := regexp.MustCompile(`\d?[-.]?\w+|-.*|"(?:\\"|[^"])+"|'(?:\\"|[^"])+'`)

}

func execCmd(commandText string, execWithSudo bool) pipe.Pipe {
	if execWithSudo {
		commandText = "sudo " + commandText
	}
	command, args := commandSplit(commandText)
	return pipe.Exec(command, args...)
}

// CommandOutput executes a command and return output bytes
func CommandOutput(commandText string, execWithSudo bool) ([]byte, error) {
	logger.WithFields(log.Fields{"cmd": commandText}).Debug("Executing command")
	commands := []pipe.Pipe{}
	commandsTextSplitted := strings.Split(commandText, ">")
	for _, cmd := range strings.Split(commandsTextSplitted[0], "|") {
		commands = append(commands, execCmd(string(strings.TrimSpace(cmd)), execWithSudo))
	}
	if len(commandsTextSplitted) > 1 {
		commands = append(commands, pipe.AppendFile(strings.TrimSpace(commandsTextSplitted[1]), 0644))
	}
	p := pipe.Line(commands...)
	outputBytes, err := pipe.Output(p)
	return outputBytes, err
}

// CommandRun executes a command
func CommandRun(commandText string, execWithSudo bool) error {
	logger.WithFields(log.Fields{"cmd": commandText}).Debug("Executing command")
	commands := []pipe.Pipe{}
	commandsTextSplitted := strings.Split(commandText, ">")
	for _, cmd := range strings.Split(commandsTextSplitted[0], "|") {
		commands = append(commands, execCmd(string(strings.TrimSpace(cmd)), execWithSudo))
	}
	if len(commandsTextSplitted) > 1 {
		commands = append(commands, pipe.AppendFile(strings.TrimSpace(commandsTextSplitted[1]), 0644))
	}
	p := pipe.Line(commands...)
	err := pipe.Run(p)
	return err
}

// CommandRunFunc executes a command and runs specified function
func CommandRunWithFunc(commandText string, execWithSudo bool, onCommand func(*pipe.State)) error {
	logger.WithFields(log.Fields{"cmd": commandText}).Debug("Executing command")
	s := pipe.NewState(nil, nil)
	commands := []pipe.Pipe{}
	commandsTextSplitted := strings.Split(commandText, ">")
	for _, cmd := range strings.Split(commandsTextSplitted[0], "|") {
		commands = append(commands, execCmd(string(strings.TrimSpace(cmd)), execWithSudo))
	}
	if len(commandsTextSplitted) > 1 {
		commands = append(commands, pipe.AppendFile(strings.TrimSpace(commandsTextSplitted[1]), 0644))
	}
	p := pipe.Line(commands...)
	err := p(s)
	if err == nil {
		onCommand(s)
		err = s.RunTasks()
	}
	return err
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
