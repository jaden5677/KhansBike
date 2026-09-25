// Command importer brings the price-list workbook into the catalogue from the
// command line, for bulk loads on the shop PC. It drives the same staged
// workflow as the web admin's import screen:
//
//	importer stage <workbook.xlsx>              stage a dry run and print the review summary
//	importer show <batch-id>                    print a batch's summary and every row with issues
//	importer commit [-public-prices] <batch-id> apply the accepted and merged rows
//	importer abort <batch-id>                   discard a batch
//
// Rows the importer could not resolve are left "pending"; decide them in the
// web admin (or skip them there) before committing.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"text/tabwriter"

	"github.com/google/uuid"

	"github.com/khansbikezone/bikezone-api/internal/app"
	"github.com/khansbikezone/bikezone-api/internal/config"
	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/importer"
)

const usage = `usage:
  importer stage <workbook.xlsx>
  importer show <batch-id>
  importer commit [-public-prices] <batch-id>
  importer abort <batch-id>`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "importer:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("missing command\n%s", usage)
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	a, err := app.New(ctx, cfg, config.NewLogger(cfg))
	if err != nil {
		return err
	}
	defer a.Close()
	im := a.Importer

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "stage":
		if len(rest) != 1 {
			return fmt.Errorf("stage needs one workbook path\n%s", usage)
		}
		f, err := os.Open(rest[0])
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		b, err := im.Stage(ctx, filepath.Base(rest[0]), f)
		if err != nil {
			return err
		}
		return show(ctx, out, im, b.ID)
	case "show":
		id, err := batchID(rest)
		if err != nil {
			return err
		}
		return show(ctx, out, im, id)
	case "commit":
		fs := flag.NewFlagSet("commit", flag.ContinueOnError)
		public := fs.Bool("public-prices", false, "publish the retail price of products this import creates")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		id, err := batchID(fs.Args())
		if err != nil {
			return err
		}
		b, err := im.Commit(ctx, id, importer.CommitOptions{RetailPricesPublic: *public})
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "committed batch %s (%s)\n", b.ID, b.Filename)
		return nil
	case "abort":
		id, err := batchID(rest)
		if err != nil {
			return err
		}
		if err := im.Abort(ctx, id); err != nil {
			return err
		}
		fmt.Fprintf(out, "aborted batch %s\n", id)
		return nil
	default:
		return fmt.Errorf("unknown command %q\n%s", cmd, usage)
	}
}

func batchID(args []string) (uuid.UUID, error) {
	if len(args) != 1 {
		return uuid.Nil, fmt.Errorf("expected one batch id\n%s", usage)
	}
	return uuid.Parse(args[0])
}

// show prints a batch's decision counts and every row that has issues.
func show(ctx context.Context, out io.Writer, im *importer.Importer, id uuid.UUID) error {
	b, err := im.Batch(ctx, id)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "batch %s  file %s  status %s\n", b.ID, b.Filename, b.Status)
	for _, d := range []domain.ImportDecision{domain.DecisionAccept, domain.DecisionMerge, domain.DecisionSkip, domain.DecisionPending} {
		fmt.Fprintf(out, "  %-8s %d\n", d, b.Counts[d])
	}
	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "\nSHEET\tROW\tDECISION\tSEVERITY\tISSUE")
	cursor := ""
	for {
		rows, next, err := im.Rows(ctx, id, importer.RowQuery{WithIssuesOnly: true, Cursor: cursor, Limit: 500})
		if err != nil {
			return err
		}
		for _, r := range rows {
			for _, is := range r.Issues {
				fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\n", r.SheetName, r.RowIndex+1, r.Decision, is.Severity, is.Message)
			}
		}
		if next == "" {
			return tw.Flush()
		}
		cursor = next
	}
}
