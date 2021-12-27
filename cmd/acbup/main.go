package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/alexcb/acbup/pack"

	goflags "github.com/jessevdk/go-flags"
)

type Flags struct {
	List   bool   `short:"l" long:"list" description:"list contents of backup"`
	Config string `short:"c" long:"config" description:"config file"`
	Help   bool   `short:"h" long:"help" description:"display this help"`
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
			panic("bad config")
			return "", "", fmt.Errorf("bad config line: %q\n", line)
		}
		key := strings.TrimSpace(fields[0])
		val := strings.TrimSpace(fields[1])

		switch key {
		case "src":
			src = val
		case "dst":
			dst = val
		default:
			return "", "", fmt.Errorf("unsupported key: %q\n", key)
		}

	}
	if src == "" {
		return "", "", fmt.Errorf("src not defined\n")
	}
	if dst == "" {
		return "", "", fmt.Errorf("dst not defined\n")
	}
	return src, dst, nil
}

func main() {
	progName := "acbup"
	if len(os.Args) > 0 {
		progName = os.Args[0]
	}
	usage := fmt.Sprintf("%s [options]", progName)

	flags := Flags{}
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

	src, dst, err := readConfig(flags.Config)
	if err != nil {
		die("failed to read config %s: %s\n", flags.Config, err)
	}

	if len(args) != 0 {
		die("unhandled args: %v", args)
	}

	p, err := pack.New(dst)
	if err != nil {
		die("failed to create new Pack: %s\n", err)
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
