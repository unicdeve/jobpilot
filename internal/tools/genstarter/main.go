// Command genstarter writes the starter list of companies JobPilot
// suggests watching (docs/02_features/17_company_watchlists.md D2,
// ADR-0024 §7).
//
// It reads each board in the seed file with the connector the product uses,
// reads each open posting with domain.ReadSignals, and keeps a board when at
// least one posting can be done from anywhere or from countries on more than
// one continent. What it measured is written with each entry, and every
// board it did not keep is reported, with why, on standard error.
//
// It is run by hand, through `make starter-list`, before each release, and
// never at build or run time: an installation never reads a board it has
// not been asked to watch.
package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/unicdev/jobpilot/internal/adapters/secondary/discovery"
	"github.com/unicdev/jobpilot/internal/core/domain"
)

// explain prints each qualifying posting, so a reviewer can see what put a
// company on the list.
var explain bool

// boardsAtOnce is how many boards are read at the same time, as the
// product reads them (spec D5).
const boardsAtOnce = 4

func main() {
	seedPath := flag.String("seed", "seed.txt", "the boards to consider, one address per line")
	out := flag.String("out", "starter_companies_gen.go", "the Go file to write")
	flag.BoolVar(&explain, "explain", false, "print each qualifying posting and how it was read")
	flag.Parse()

	boards, err := readSeed(*seedPath)
	if err != nil {
		log.Fatal(err)
	}

	reader := discovery.NewCompanyBoards(discovery.DefaultBoardEndpoints)
	today := time.Now().UTC().Format("2006-01-02")
	measured := measureAll(context.Background(), reader, boards, today)

	var kept []domain.StarterCompany
	for i, measurement := range measured {
		switch {
		case measurement.err != nil:
			fmt.Fprintf(os.Stderr, "dropped %s: %v\n", boards[i].URL(), measurement.err)
		case measurement.company.Qualifying == 0:
			fmt.Fprintf(os.Stderr, "dropped %s: no open role of %d could be done from anywhere or from more than one continent\n",
				boards[i].URL(), measurement.company.OpenRoles)
		default:
			kept = append(kept, measurement.company)
		}
	}
	// Most qualifying roles first: the suggestions a candidate reads first
	// are the ones with the most to offer them.
	sort.Slice(kept, func(i, j int) bool {
		if kept[i].Qualifying != kept[j].Qualifying {
			return kept[i].Qualifying > kept[j].Qualifying
		}
		return strings.ToLower(kept[i].Name) < strings.ToLower(kept[j].Name)
	})

	source, err := render(kept, today)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, source, 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Fprintf(os.Stderr, "kept %d of %d boards in %s\n", len(kept), len(boards), *out)
}

// readSeed reads the boards to consider: one address per line, with blank
// lines and lines starting with # ignored.
func readSeed(path string) ([]domain.Board, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var boards []domain.Board
	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		board, err := domain.ParseBoard(text)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		for _, seen := range boards {
			if seen.Same(board) {
				return nil, fmt.Errorf("%s:%d: %s is listed twice", path, line, text)
			}
		}
		boards = append(boards, board)
	}
	return boards, scanner.Err()
}

type measurement struct {
	company domain.StarterCompany
	err     error
}

func measureAll(ctx context.Context, reader *discovery.CompanyBoards, boards []domain.Board, today string) []measurement {
	results := make([]measurement, len(boards))
	slots := make(chan struct{}, boardsAtOnce)
	var wg sync.WaitGroup
	for i, board := range boards {
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			results[i] = measure(ctx, reader, board, today)
		})
	}
	wg.Wait()
	return results
}

// measure reads one board and counts its postings that qualify.
func measure(ctx context.Context, reader *discovery.CompanyBoards, board domain.Board, today string) measurement {
	contents, err := reader.ReadBoard(ctx, board)
	if err != nil {
		return measurement{err: err}
	}
	name, err := reader.BoardName(ctx, board)
	if err != nil {
		return measurement{err: fmt.Errorf("reading its name: %w", err)}
	}
	if name = domain.CleanCompanyName(name); name == "" {
		return measurement{err: errors.New("its board gives no company name, so a suggestion could not say whose it is")}
	}

	jobs := contents.Jobs
	if contents.Undescribed {
		// A description can make where a posting can be done from less
		// certain, never wider (ADR-0023 §1), so only the postings that
		// qualify without one need describing (ADR-0026 §2).
		var candidates []domain.Job
		for _, job := range jobs {
			domain.ReadSignals(&job)
			if domain.OpenAcrossContinents(job.Workable) {
				candidates = append(candidates, job)
			}
		}
		if jobs, err = reader.DescribeJobs(ctx, board, candidates); err != nil {
			return measurement{err: fmt.Errorf("describing its postings: %w", err)}
		}
	}

	company := domain.StarterCompany{Board: board, Name: name, MeasuredOn: today, OpenRoles: len(contents.Jobs)}
	for _, job := range jobs {
		domain.ReadSignals(&job)
		if domain.OpenAcrossContinents(job.Workable) {
			company.Qualifying++
			if explain {
				fmt.Fprintf(os.Stderr, "%s: %q %s %q → %s %v\n", name, job.Title, job.RemoteType, job.Locations,
					job.Workable.Scope, job.Workable.Places)
			}
		}
	}
	return measurement{company: company}
}

func render(companies []domain.StarterCompany, today string) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("// Code generated by genstarter. DO NOT EDIT.\n//\n")
	fmt.Fprintf(&b, "// Measured on %s. Regenerate with `make starter-list`.\n\n", today)
	b.WriteString("package domain\n\n")
	b.WriteString("// starterCompanies is every seed board with at least one open role that\n")
	b.WriteString("// could be done from anywhere or from more than one continent.\n")
	b.WriteString("var starterCompanies = []StarterCompany{\n")
	for _, company := range companies {
		region := ""
		if company.Board.Region != "" {
			region = fmt.Sprintf("Region: %q, ", company.Board.Region)
		}
		fmt.Fprintf(&b, "\t{Board: Board{Provider: %q, %sName: %q}, Name: %q, MeasuredOn: %q, OpenRoles: %d, Qualifying: %d},\n",
			company.Board.Provider, region, company.Board.Name, company.Name, company.MeasuredOn, company.OpenRoles, company.Qualifying)
	}
	b.WriteString("}\n")
	return format.Source(b.Bytes())
}
