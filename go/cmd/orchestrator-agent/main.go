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

package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	prefixed "github.com/MaxFedotov/logrus-prefixed-formatter"
	"github.com/github/orchestrator-agent/go/agent"
	log "github.com/sirupsen/logrus"
)

var AppVersion string

func acceptSignal() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill, syscall.SIGHUP)

	// Block until a signal is received.
	sig := <-c
	log.Fatalf("Got signal: %+v", sig)
}

func init() {
	log.SetFormatter(&prefixed.TextFormatter{FullTimestamp: true, ForceFormatting: true})
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)
}

// main is the application's entry point. It will either spawn a CLI or HTTP interfaces.
func main() {
	configFile := flag.String("config", "/etc/orchestrator-agent.conf", "config file name")
	printVersion := flag.Bool("version", false, "Print version")
	flag.Parse()

	if AppVersion == "" {
		AppVersion = "local-build"
	}

	if *printVersion {
		fmt.Print(AppVersion)
		return
	}

	defaultLogger := log.WithFields(log.Fields{"prefix": "agent"})

	app := agent.New(*configFile, defaultLogger)

	if err := app.LoadConfig(); err != nil {
		defaultLogger.WithField("config", *configFile).Fatal(err)
	}

	defaultLogger.WithField("version", AppVersion).Info("Starting orchestrator-agent")

	if err := app.Start(); err != nil {
		defaultLogger.WithField("error", err).Fatal("Unable to initialize orchestrator-agent")
	}

	defaultLogger.WithField("token", app.Info.Token).Info("Process token generated")

	acceptSignal()

	//TODO
	// gracefull shutdown
}
