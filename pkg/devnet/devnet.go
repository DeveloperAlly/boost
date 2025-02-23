package devnet

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("devnet")

func Run(ctx context.Context, tempHome string, done chan struct{}) {
	var wg sync.WaitGroup

	log.Debugw("using temp home dir", "dir", tempHome)

	// The parameter files can be as large as 1GiB.
	// If this is the first time lotus runs,
	// and the machine doesn't have particularly fast internet,
	// we don't want devnet to seemingly stall for many minutes.
	// Instead, show the download progress explicitly.
	// fetch-params will exit in about a second if all files are up to date.
	// The command is also pretty verbose, so reduce its verbosity.
	{
		// Ten minutes should be enough for practically any machine.
		ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)

		log.Debugw("lotus fetch-params 8388608")
		cmd := exec.CommandContext(ctx, "lotus", "fetch-params", "8388608")
		cmd.Env = []string{fmt.Sprintf("HOME=%s", tempHome), "GOLOG_LOG_LEVEL=error"}
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Fatal(err)
		}
		cancel()
	}

	wg.Add(2)
	go func() {
		runLotusDaemon(ctx, tempHome)
		log.Debugw("shut down lotus daemon")
		wg.Done()
	}()

	go func() {
		runLotusMiner(ctx, tempHome)
		log.Debugw("shut down lotus miner")
		wg.Done()
	}()

	//TODO: Fix setDefaultWalletCmd to work with a temporary $HOME
	//go func() {
	//setDefaultWalletCmd(ctx, tempHome)
	//wg.Done()
	//}()

	wg.Wait()

	done <- struct{}{}
}

func runCmdsWithLog(ctx context.Context, name string, commands [][]string, homeDir string) {
	logFile, err := os.Create(name + ".log")
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()

	for _, cmdArgs := range commands {
		log.Debugw("running command", "name", name, "cmd", strings.Join(cmdArgs, " "))
		cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		cmd.Env = []string{fmt.Sprintf("HOME=%s", homeDir)}
		// If ctx.Err()!=nil, we cancelled the command via SIGINT.
		if err := cmd.Run(); err != nil && ctx.Err() == nil {
			log.Errorw("check logfile for details", "err", err, "logfile", logFile.Name())
			break
		}
	}
}

func runLotusDaemon(ctx context.Context, home string) {
	cmds := [][]string{
		{"lotus-seed", "genesis", "new", "localnet.json"},
		{"lotus-seed", "pre-seal", "--sector-size=8388608", "--num-sectors=1"},
		{"lotus-seed", "genesis", "add-miner", "localnet.json",
			filepath.Join(home, ".genesis-sectors", "pre-seal-t01000.json")},
		{"lotus", "daemon", "--lotus-make-genesis=dev.gen",
			"--genesis-template=localnet.json", "--bootstrap=false"},
	}

	runCmdsWithLog(ctx, "lotus-daemon", cmds, home)
}

func runLotusMiner(ctx context.Context, home string) {
	cmds := [][]string{
		{"lotus", "wait-api"}, // wait for lotus node to run

		{"lotus", "wallet", "import",
			filepath.Join(home, ".genesis-sectors", "pre-seal-t01000.key")},
		{"lotus-miner", "init", "--genesis-miner", "--actor=t01000", "--sector-size=8388608",
			"--pre-sealed-sectors=" + filepath.Join(home, ".genesis-sectors"),
			"--pre-sealed-metadata=" + filepath.Join(home, ".genesis-sectors", "pre-seal-t01000.json"),
			"--nosync"},

		// Starting in network version 13,
		// pre-commits are batched by default,
		// and commits are aggregated by default.
		// This means deals could sit at StorageDealAwaitingPreCommit or
		// StorageDealSealing for a while, going past our 10m test timeout.
		{"sed", "-Ei", "-e", "s/#BatchPreCommits\\ =\\ true/BatchPreCommits=false/",
			filepath.Join(home, ".lotusminer", "config.toml")},

		{"sed", "-Ei", "-e", "s/#AggregateCommits\\ =\\ true/AggregateCommits=false/",
			filepath.Join(home, ".lotusminer", "config.toml")},

		{"sed", "-Ei", "-e", "s/#EnableMarkets\\ =\\ true/EnableMarkets=false/",
			filepath.Join(home, ".lotusminer", "config.toml")},

		{"lotus-miner", "run", "--nosync"},
	}

	runCmdsWithLog(ctx, "lotus-miner", cmds, home)
}

//func setDefaultWalletCmd(ctx context.Context, _ string) {
//// TODO: do this without a shell
//setDefaultWalletCmd := "lotus wallet list | grep t3 | awk '{print $1}' | xargs lotus wallet set-default"

//for {
//select {
//case <-ctx.Done():
//return
//case <-time.After(5 * time.Second):
//}

//cmd := exec.CommandContext(ctx, "sh", "-c", setDefaultWalletCmd)
//_, err := cmd.CombinedOutput()
//if err != nil {
//continue
//}
//// TODO: stop once we've set the default wallet once.
//}
//}

func GetMinerEndpoint(ctx context.Context, homedir string) (string, error) {
	cmdArgs := []string{"lotus-miner", "auth", "api-info", "--perm=admin"}

	var out bytes.Buffer

	log.Debugw("getting auth token", "command", strings.Join(cmdArgs, " "))
	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	cmd.Env = []string{fmt.Sprintf("HOME=%s", homedir)}
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}

	ai := strings.TrimPrefix(strings.TrimSpace(out.String()), "MINER_API_INFO=")
	ai = strings.TrimSuffix(ai, "\n")

	return ai, nil
}

func GetFullnodeEndpoint(ctx context.Context, homedir string) (string, error) {
	cmdArgs := []string{"lotus", "auth", "api-info", "--perm=admin"}

	var out bytes.Buffer

	log.Debugw("getting auth token", "command", strings.Join(cmdArgs, " "))
	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	cmd.Env = []string{fmt.Sprintf("HOME=%s", homedir)}
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}

	ai := strings.TrimPrefix(strings.TrimSpace(out.String()), "FULLNODE_API_INFO=")
	ai = strings.TrimSuffix(ai, "\n")

	return ai, nil
}
