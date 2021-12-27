package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/alexcb/acbup/pack"
	"github.com/alexcb/acbup/util/termutil"

	goflags "github.com/jessevdk/go-flags"
)

type flags struct {
	Recover bool   `long:"recover" description:"attempt to fix corrupted data"`
	Restore bool   `long:"restore-local-file-from-backup" description:"overwrites local file from backed up copy"`
	Verify  bool   `long:"verify" description:"verify backup integrity"`
	List    bool   `short:"l" long:"list" description:"list contents of backup"`
	Config  string `short:"c" long:"config" description:"config file"`
	Help    bool   `short:"h" long:"help" description:"display this help"`
}

func die(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg, args...)
	os.Exit(1)
}

func readConfig(path string) (string, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer file.Close()

	var src string
	var dst string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		fields := strings.SplitN(line, "=", 2)
		if len(fields) < 2 {
			return "", "", fmt.Errorf("bad config line: %q", line)
		}
		key := strings.TrimSpace(fields[0])
		val := strings.TrimSpace(fields[1])

		switch key {
		case "src":
			src = val
		case "dst":
			dst = val
		default:
			return "", "", fmt.Errorf("unsupported key: %q", key)
		}

	}
	if src == "" {
		return "", "", fmt.Errorf("src not defined")
	}
	if dst == "" {
		return "", "", fmt.Errorf("dst not defined")
	}
	return src, dst, nil
}

func main() {
	progName := "acbup"
	if len(os.Args) > 0 {
		progName = os.Args[0]
	}
	usage := fmt.Sprintf("%s [options]", progName)

	flags := flags{}
	parser := goflags.NewNamedParser("", goflags.PrintErrors|goflags.PassDoubleDash|goflags.PassAfterNonOption)
	parser.AddGroup(usage, "", &flags)
	args, err := parser.ParseArgs(os.Args[1:])
	if err != nil {
		die("failed to parse flags: %s\n", err)
	}
	if flags.Help {
		parser.WriteHelp(os.Stdout)
		os.Exit(0)
	}

	if flags.Config == "" {
		die("no config file was given\n")
	}

	src, dst, err := readConfig(flags.Config)
	if err != nil {
		die("failed to read config %s: %s\n", flags.Config, err)
	}

	interactive := termutil.IsTTY()

	if flags.Verify {
		if len(args) != 0 {
			die("unhandled args: %v", args)
		}

		p, err := pack.New(dst, true, interactive)
		if err != nil {
			die("failed to create new Pack: %s\n", err)
		}

		ok := p.Verify()
		if !ok {
			die("verification of %s failed\n", dst)
		}
		fmt.Printf("verification of %s passed\n", dst)
		return
	}

	p, err := pack.New(dst, false, interactive)
	if err != nil {
		die("failed to create new Pack: %s\n", err)
	}

	if flags.Restore {
		if len(args) == 0 {
			die("restore takes one or more local filepaths to restore")
		}
		for _, path := range args {
			err := p.Restore(path)
			if err != nil {
				die("restore-local-file-from-backup of %s failed: %s\n", path, err)
			}
			fmt.Printf("restore-local-file-from-backup of %s done\n", path)
		}
		return
	}

	if len(args) != 0 {
		die("unhandled args: %v", args)
	}

	if flags.Recover {
		// TODO recovery mode should only perform recovery under p.Recover() and never under pack.New()
		// in fact we should move this logic into a function (rather than method): pack.Recover(dst)
		numOK, numRecovered, numFailed, err := p.Recover()
		if err != nil {
			die("recovery of %s failed: %s\n", dst, err)
		}
		if numFailed > 0 {
			die("recovery of %s failed to recover %d file(s) (%d file(s) were recovered, %d file(s) were OK)\n", dst, numFailed, numRecovered, numOK)
		}

		fmt.Printf("recovery of %s passed: %d corrupt file(s) were recovered (%d file(s) were OK)\n", dst, numRecovered, numOK)
		return
	}

	if flags.List {
		files, err := p.List()
		if err != nil {
			die("failed to list contents of backup %s: %s\n", dst, err)
		}
		for _, f := range files {
			fmt.Println(f)
		}
		return
	}

	err = p.AddDir(src)
	if err != nil {
		die("failed to add dir %s: %s\n", src, err)
	}
	fmt.Printf("done\n")

	err = p.Close()
	if err != nil {
		die("failed to close pack %s: %s\n", dst, err)
	}
}
