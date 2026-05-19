package main

import "flag"

type cliFlags struct {
	filter      string
	config      string
	concurrency int
	compare     bool
	promptFile  string
	evalID      string
	model       string // comma-separated model list for targeted mode
	allModels   bool   // expand to eval's primary + secondaries
}

func parseFlags() cliFlags {
	var f cliFlags
	flag.StringVar(&f.filter, "filter", "", "filter evals by ID prefix or tests: field value")
	flag.StringVar(&f.config, "config", ".skill-eval.yml", "path to .skill-eval.yml config file")
	// -1 sentinel: flag not provided; use config value.
	flag.IntVar(&f.concurrency, "concurrency", -1, "number of concurrent evals (default: from config, fallback 1)")
	flag.BoolVar(&f.compare, "compare", false, "run each eval twice (with and without prompt) and classify")
	flag.StringVar(&f.promptFile, "prompt-file", "", "targeted mode: run all evals whose prompt_file matches this path")
	flag.StringVar(&f.evalID, "eval", "", "targeted mode: run the eval with this ID")
	flag.StringVar(&f.model, "model", "", "targeted mode: comma-separated list of models to run against")
	flag.BoolVar(&f.allModels, "all-models", false, "targeted mode: use the eval's primary + secondaries model list")
	flag.Parse()
	return f
}
