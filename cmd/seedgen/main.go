package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/hellprise/interline-engine/internal/seed"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "seedgen: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	seedVal := flag.Uint64("seed", 42, "PRNG seed")
	startStr := flag.String("start", "", "first service day in YYYY-MM-DD (UTC)")
	days := flag.Int("days", 30, "number of days to generate")
	outDir := flag.String("out", "", "output directory for CSV files")

	flag.Parse()

	if flag.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %v", flag.Args())
	}
	if *startStr == "" {
		return errors.New("missing -start date")
	}
	if *outDir == "" {
		return errors.New("missing -out directory")
	}

	start, err := time.Parse(time.DateOnly, *startStr)
	if err != nil {
		return fmt.Errorf("invalid start date %q: %w", *startStr, err)
	}

	data, err := seed.Generate(seed.Config{
		Start: start,
		Seed:  *seedVal,
		Days:  *days,
	})
	if err != nil {
		return err
	}

	return seed.WriteCSV(*outDir, data)
}
