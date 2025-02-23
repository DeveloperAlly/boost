package main

import (
	"os"

	logging "github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/boost/build"
	cliutil "github.com/filecoin-project/boost/cli/util"
)

var log = logging.Logger("boostd")

const (
	FlagBoostRepo = "boost-repo"
)

func main() {
	app := &cli.App{
		Name:                 "boostd",
		Usage:                "Markets V2 module for Filecoin",
		EnableBashCompletion: true,
		Version:              build.UserVersion(),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    FlagBoostRepo,
				EnvVars: []string{"BOOST_PATH"},
				Usage:   "boost repo path",
				Value:   "~/.boost",
			},
			cliutil.FlagVeryVerbose,
		},
		Commands: []*cli.Command{
			authCmd,
			runCmd,
			initCmd,
			migrateCmd,
			dummydealCmd,
			storageDealsCmd,
			dataTransfersCmd,
			retrievalDealsCmd,
			indexProvCmd,
			offlineDealCmd,
			logCmd,
			dagstoreCmd,
		},
	}
	app.Setup()

	if err := app.Run(os.Args); err != nil {
		os.Stderr.WriteString("Error: " + err.Error() + "\n")
	}
}

func before(cctx *cli.Context) error {
	_ = logging.SetLogLevel("boostd", "INFO")
	_ = logging.SetLogLevel("db", "INFO")

	if cliutil.IsVeryVerbose {
		_ = logging.SetLogLevel("boostd", "DEBUG")
		_ = logging.SetLogLevel("provider", "DEBUG")
		_ = logging.SetLogLevel("gql", "DEBUG")
		_ = logging.SetLogLevel("boost-provider", "DEBUG")
		_ = logging.SetLogLevel("storagemanager", "DEBUG")
		_ = logging.SetLogLevel("index-provider-wrapper", "DEBUG")
		_ = logging.SetLogLevel("boost-migrator", "DEBUG")
		_ = logging.SetLogLevel("dagstore", "DEBUG")
		_ = logging.SetLogLevel("migrator", "DEBUG")
	}

	return nil
}
