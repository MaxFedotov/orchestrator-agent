package cmd

import (
	"fmt"
	"regexp"
	"strings"

	log "github.com/sirupsen/logrus"
	"gopkg.in/pipe.v2"
)

type CmdOpts struct {
	execWithSudo bool
	sudoUser     string
	logger       *log.Entry
}

func NewCmd(execWithSudo bool, sudoUser string, logger *log.Entry) *CmdOpts {
	return &CmdOpts{execWithSudo: execWithSudo, sudoUser: sudoUser, logger: logger}
}

func (c *CmdOpts) commandSplit(commandText string) (string, []string) {
	var args []string
	cmd := ""
	//re := regexp.MustCompile(`(?:\S*".*?")|[-./.\d\w]\S*\d?|"(?:\\"|[^"])+"|'(?:\\"|[^"])+'`)
	re := regexp.MustCompile(`(?:\S*'.*?')|(?:\S*".*?")|[-./.\d\w]\S*\d?|"(?:\\"|[^"])+"|'(?:\\"|[^"])+'`)
	res := re.FindAllStringSubmatch(commandText, -1)

	for idx, match := range res {
		for _, arg := range match {
			if idx == 0 {
				cmd = arg
			} else {
				if arg == "' '" || arg == "\" \"" {
					args = append(args, " ")
				} else {
					if strings.HasPrefix(arg, "\"") && strings.HasSuffix(arg, "\"") {
						arg = strings.Trim(arg, "\"")
					}
					if strings.HasPrefix(arg, "'") && strings.HasSuffix(arg, "'") {
						arg = strings.Trim(arg, "'")
					}
					args = append(args, arg)
				}
			}
		}
	}
	return cmd, args
}

func (c *CmdOpts) execCmd(commandText string) pipe.Pipe {
	if c.execWithSudo {
		if len(c.sudoUser) > 0 {
			commandText = fmt.Sprintf("sudo -u %s %s", c.sudoUser, commandText)
		} else {
			commandText = "sudo " + commandText
		}
	}
	command, args := c.commandSplit(commandText)
	return pipe.Exec(command, args...)
}

// CommandOutput executes a command and return output bytes
func (c *CmdOpts) CommandOutput(commandText string) ([]byte, error) {
	c.logger.WithFields(log.Fields{"cmd": commandText}).Debug("Executing command")
	commands := []pipe.Pipe{}
	commandsTextSplitted := strings.Split(commandText, ">")
	for _, cmd := range strings.Split(commandsTextSplitted[0], "|") {
		commands = append(commands, c.execCmd(string(strings.TrimSpace(cmd))))
	}
	if len(commandsTextSplitted) > 1 {
		commands = append(commands, pipe.AppendFile(strings.TrimSpace(commandsTextSplitted[1]), 0644))
	}
	p := pipe.Line(commands...)
	outputBytes, stderr, err := pipe.DividedOutput(p)
	if err != nil {
		return nil, fmt.Errorf("error: %+v, stderr: %s", err, string(stderr))
	}
	return outputBytes, nil
}

// CommandCombinedOutput executes a command and return output bytes (stderr and stdout)
func (c *CmdOpts) CommandCombinedOutput(commandText string) ([]byte, error) {
	c.logger.WithFields(log.Fields{"cmd": commandText}).Debug("Executing command")
	commands := []pipe.Pipe{}
	commandsTextSplitted := strings.Split(commandText, ">")
	for _, cmd := range strings.Split(commandsTextSplitted[0], "|") {
		commands = append(commands, c.execCmd(string(strings.TrimSpace(cmd))))
	}
	if len(commandsTextSplitted) > 1 {
		commands = append(commands, pipe.AppendFile(strings.TrimSpace(commandsTextSplitted[1]), 0644))
	}
	p := pipe.Line(commands...)
	outputBytes, err := pipe.CombinedOutput(p)
	return outputBytes, err
}

// CommandRun executes a command
func (c *CmdOpts) CommandRun(commandText string) error {
	c.logger.WithFields(log.Fields{"cmd": commandText}).Debug("Executing command")
	commands := []pipe.Pipe{}
	commandsTextSplitted := strings.Split(commandText, ">")
	for _, cmd := range strings.Split(commandsTextSplitted[0], "|") {
		commands = append(commands, c.execCmd(string(strings.TrimSpace(cmd))))
	}
	if len(commandsTextSplitted) > 1 {
		commands = append(commands, pipe.AppendFile(strings.TrimSpace(commandsTextSplitted[1]), 0644))
	}
	p := pipe.Line(commands...)
	_, stderr, err := pipe.DividedOutput(p)
	if err != nil {
		return fmt.Errorf("error: %+v, stderr: %s", err, string(stderr))
	}
	return nil
}

// CommandRunWithFunc executes a command and runs specified function
func (c *CmdOpts) CommandRunWithFunc(commandText string, onCommand func(*pipe.State)) error {
	c.logger.WithFields(log.Fields{"cmd": commandText}).Debug("Executing command")
	outb := &pipe.OutputBuffer{}
	s := pipe.NewState(nil, outb)
	commands := []pipe.Pipe{}
	commandsTextSplitted := strings.Split(commandText, ">")
	for _, cmd := range strings.Split(commandsTextSplitted[0], "|") {
		commands = append(commands, c.execCmd(string(strings.TrimSpace(cmd))))
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
	if err != nil {
		return fmt.Errorf("error: %+v, stderr: %s", err, string(outb.Bytes()))
	}
	return nil
}

func (c *CmdOpts) OutputLines(commandOutput []byte) []string {
	text := strings.Trim(fmt.Sprintf("%s", commandOutput), "\n")
	lines := strings.Split(text, "\n")
	return lines
}

func (c *CmdOpts) OutputTokens(delimiterPattern string, commandOutput []byte) [][]string {
	lines := c.OutputLines(commandOutput)
	tokens := make([][]string, len(lines))
	for i := range tokens {
		tokens[i] = regexp.MustCompile(delimiterPattern).Split(lines[i], -1)
	}
	return tokens
}
