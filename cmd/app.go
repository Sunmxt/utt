package cmd

import (
	"fmt"
	"os"

	"git.uestc.cn/sunmxt/utt/config"
	"github.com/jinzhu/configor"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

// App is UTT application instance.
type App struct {
	*cli.App

	ConfigFile string
	cfg        *config.Daemon

	Retry int
}

// NewApp create UTT application instance.
func NewApp() (a *App) {
	a = &App{
		cfg: &config.Daemon{},
	}
	a.App = &cli.App{
		Name:  "utt",
		Usage: "Overlay network router, designed for connecting cloud network infrastructure",
		Commands: []*cli.Command{
			newEdgeCmd(a),
			newNetworkCmd(a),
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "config",
				Aliases:     []string{"c"},
				Usage:       "config file",
				Destination: &a.ConfigFile,
				DefaultText: "/etc/utt.yml",
			},
		},
		Before: func(ctx *cli.Context) (err error) {
			if a.ConfigFile == "" {
				a.ConfigFile = "/etc/utt.yml"
			}
			// config file is a must.
			if fileInfo, err := os.Stat(a.ConfigFile); err != nil || !fileInfo.Mode().IsRegular() {
				return cmdError("invalid configuration file: %v", err)
			}
			if err = configor.New(&configor.Config{
				Debug: false,
			}).Load(a.cfg, a.ConfigFile); err != nil {
				return cmdError("failed to load configuration: %v", err)
			}
			return nil
		},
	}
	return
}

func cmdError(format string, args ...interface{}) error {
	err := fmt.Errorf(format, args...)
	log.Error(err)
	return err
}