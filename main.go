package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

type Options struct {
	Root         string
	ConfigPath   string
	Format       string
	OutputPath   string
	ListFiles    bool
	SortMode     string
	AllFiles     bool
	Include      []string
	Exclude      []string
	DropLargest  int
	ShowClusters bool
}

type patternList []string

func (p *patternList) String() string {
	return strings.Join(*p, ",")
}

func (p *patternList) Set(value string) error {
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			*p = append(*p, item)
		}
	}
	return nil
}

func main() {
	opts, err := parseOptions(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		log.Fatal(err)
	}

	config, err := loadConfig(opts.ConfigPath)
	if err != nil {
		log.Fatalf("Unable to load config: %v", err)
	}

	result, err := analyze(opts, config)
	if err != nil {
		log.Fatal(err)
	}

	if err := writeResult(os.Stdout, opts, result); err != nil {
		log.Fatal(err)
	}
}

func parseOptions(args []string) (Options, error) {
	opts := Options{
		ConfigPath:   "languages.json",
		Format:       "text",
		SortMode:     "none",
		ShowClusters: true,
	}

	var includes patternList
	var excludes patternList
	var noClusters bool
	fs := flag.NewFlagSet("codesiz", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: codesiz [options] <path>")
		fmt.Fprintln(fs.Output())
		fmt.Fprintln(fs.Output(), "Options:")
		fmt.Fprintln(fs.Output(), "  --all                 scan all regular files instead of profile includes")
		fmt.Fprintln(fs.Output(), "  --config <file>       scan profile JSON file (default \"languages.json\")")
		fmt.Fprintln(fs.Output(), "  --drop-largest <n>    exclude the largest n files from analysis")
		fmt.Fprintln(fs.Output(), "  --exclude <pattern>   exclude glob pattern; repeat or use commas")
		fmt.Fprintln(fs.Output(), "  --format <format>     output format: text, json, histogram (default \"text\")")
		fmt.Fprintln(fs.Output(), "  --histogram           alias for --format histogram")
		fmt.Fprintln(fs.Output(), "  --include <pattern>   include glob pattern; repeat or use commas")
		fmt.Fprintln(fs.Output(), "  --list                include per-file details")
		fmt.Fprintln(fs.Output(), "  --no-clusters         hide k-means file size clusters")
		fmt.Fprintln(fs.Output(), "  --output <file>       write output to a file instead of stdout")
		fmt.Fprintln(fs.Output(), "  --sort <mode>         sort file details: none, path, lines (default \"none\")")
		fmt.Fprintln(fs.Output(), "  -h, --help            print help")
		fmt.Fprintln(fs.Output(), "  -?                    print help")
	}

	fs.StringVar(&opts.ConfigPath, "config", opts.ConfigPath, "scan profile JSON file")
	fs.StringVar(&opts.Format, "format", opts.Format, "output format: text, json, histogram")
	fs.StringVar(&opts.OutputPath, "output", "", "write output to a file instead of stdout")
	fs.BoolVar(&opts.ListFiles, "list", false, "include per-file details")
	fs.StringVar(&opts.SortMode, "sort", opts.SortMode, "sort file details: none, path, lines")
	fs.BoolVar(&opts.AllFiles, "all", false, "scan all regular files instead of profile includes")
	fs.Var(&includes, "include", "include glob pattern; repeat or use commas")
	fs.Var(&excludes, "exclude", "exclude glob pattern; repeat or use commas")
	fs.IntVar(&opts.DropLargest, "drop-largest", 0, "exclude the largest n files from analysis")
	fs.BoolVar(&noClusters, "no-clusters", false, "hide k-means file size clusters")

	fs.BoolFunc("?", "print help", func(string) error {
		fs.Usage()
		return flag.ErrHelp
	})
	fs.BoolFunc("histogram", "alias for --format histogram", func(string) error {
		opts.Format = "histogram"
		return nil
	})

	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if fs.NArg() < 1 {
		return opts, fmt.Errorf("please provide a folder name")
	}
	if fs.NArg() > 1 {
		return opts, fmt.Errorf("expected one folder argument, got %d", fs.NArg())
	}

	opts.Root = fs.Arg(0)
	opts.Include = includes
	opts.Exclude = excludes
	opts.ShowClusters = !noClusters

	opts.Format = strings.ToLower(opts.Format)
	opts.SortMode = strings.ToLower(opts.SortMode)
	switch opts.Format {
	case "text", "json", "histogram":
	default:
		return opts, fmt.Errorf("unsupported format %q", opts.Format)
	}
	switch opts.SortMode {
	case "none", "path", "lines":
	default:
		return opts, fmt.Errorf("unsupported sort %q", opts.SortMode)
	}
	if opts.DropLargest < 0 {
		return opts, fmt.Errorf("--drop-largest cannot be negative")
	}
	if opts.Format == "histogram" {
		opts.ListFiles = true
	}
	return opts, nil
}
