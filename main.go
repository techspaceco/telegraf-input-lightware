package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	_ "github.com/techspaceco/telegraf-input-lightware/plugins/inputs/lightware"

	"github.com/influxdata/telegraf/plugins/common/shim"
)

var pollInterval = flag.Duration("poll_interval", 30*time.Second, "how often to send metrics")
var pollIntervalDisabled = flag.Bool("poll_interval_disabled", false, "set to true to disable polling. You want to use this when you are sending metrics on your own schedule")
var configFile = flag.String("config", "", "path to the config file for this plugin")
var err error

func main() {
	flag.Parse()
	if *pollIntervalDisabled {
		*pollInterval = shim.PollIntervalDisabled
	}

	shim := shim.New()
	err = shim.LoadConfig(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading input: %s\n", err)
		os.Exit(1)
	}

	if err := shim.Run(*pollInterval); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}
