package main

import (
	"os"

	"github.com/kimchelly/treebot-go/operations"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

const logFilesFlag = "log-files"

func main() {
	app := cli.NewApp()
	app.Name = "treebot"
	app.Usage = "integration between Dependabot and Evergreen"
	app.Commands = []*cli.Command{
		operations.AutoAuthorize(),
		operations.AutoMerge(),
	}
	app.Flags = []cli.Flag{
		&cli.StringSliceFlag{
			Name:  logFilesFlag,
			Usage: "file path(s) where output will be written",
		},
	}
	app.Before = func(c *cli.Context) error {
		logFiles := c.StringSlice(logFilesFlag)
		if len(logFiles) == 0 {
			l, err := zap.NewDevelopment()
			if err != nil {
				return errors.Wrap(err, "building std logger")
			}
			zap.ReplaceGlobals(l)
			return nil
		}

		conf := zap.NewDevelopmentConfig()
		conf.OutputPaths = logFiles
		l, err := conf.Build()
		if err != nil {
			return errors.Wrap(err, "building file logger")
		}
		zap.ReplaceGlobals(l)

		return nil
	}
	app.EnableBashCompletion = true
	if err := app.Run(os.Args); err != nil {
		zap.S().Error(err)
		os.Exit(1)
	}
}
