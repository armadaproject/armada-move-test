package main

import (
	"github.com/G-Research/armada/internal/common"
	"github.com/G-Research/armada/internal/lookoutingesterv2/benchmark"
	configuration "github.com/G-Research/armada/internal/lookoutingesterv2/configuration"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const CustomConfigLocation string = "config"

func init() {
	pflag.StringSlice(
		CustomConfigLocation,
		[]string{},
		"Fully qualified path to application configuration file (for multiple config files repeat this arg or separate paths with commas)",
	)
	pflag.Parse()
}

func main() {
	common.ConfigureLogging()
	common.BindCommandlineArguments()

	var config configuration.LookoutIngesterV2Configuration
	userSpecifiedConfigs := viper.GetStringSlice(CustomConfigLocation)

	common.LoadConfig(&config, "./config/lookoutingesterv2", userSpecifiedConfigs)

	benchmark.RunBenchmark(config)
}
