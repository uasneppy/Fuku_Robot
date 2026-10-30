// Command claude-spike is the owner's spike tool for the raid AI image checks.
// It sends labelled sample images through the same alita/utils/claude client
// the bot uses and prints accuracy, latency, tokens and an estimated cost, so
// the owner can check the request shape and the 0.85 bar against the live
// Anthropic API before relying on it.
//
// It reads ANTHROPIC_API_KEY from the environment only, never from a flag, and
// never prints it. Every sample image is sent to Anthropic during a run. The
// tool is not part of the bot and imports nothing from the repository except
// the standard-library-only claude package, so it never triggers the database
// connection that alita/config and alita/db make before main.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/divkix/Alita_Robot/alita/utils/claude"
)

const (
	envKey = "ANTHROPIC_API_KEY"

	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2

	maxConcurrency = 8
	maxTokensLimit = 4096
	sniffBytes     = 512
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr, http.DefaultTransport))
}

// config is the parsed command line.
type config struct {
	dir         string
	model       string
	endpoint    string
	csvPath     string
	bar         float64
	concurrency int
	limit       int
	priceIn     float64
	priceOut    float64
	probe       bool
	modelsGet   bool
	fakeKey     bool
	options     claude.Options
}

// rawFlags are the option flags that need a check after parsing.
type rawFlags struct {
	toolChoice string
	thinking   string
}

// run is main without the process: it parses args, reads the environment
// through getenv, writes to stdout and stderr and makes every request through
// transport. It returns the exit code. Nothing is sent before the API key has
// been found.
func run(args []string, getenv func(string) string, stdout, stderr io.Writer, transport http.RoundTripper) int {
	cfg, flags, raw, code := parseFlags(args, stderr)
	if code >= 0 {
		return code
	}

	key := getenv(envKey)
	if key == "" && cfg.probe && cfg.fakeKey {
		key = fakeAPIKey
	}
	if key == "" {
		_, _ = fmt.Fprintf(stderr, "%s is not set, so nothing was sent.\n\n", envKey)
		usage(stderr, flags)
		return exitUsage
	}

	if msg := validate(cfg, raw); msg != "" {
		_, _ = fmt.Fprintf(stderr, "claude-spike: %s\n", msg)
		return exitUsage
	}

	client := &claude.Client{
		Endpoint: cfg.endpoint,
		APIKey:   key,
		Model:    cfg.model,
		HTTP: &http.Client{
			Transport: transport,
			Timeout:   claude.Timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		Options: cfg.options,
	}

	switch {
	case cfg.probe:
		return runProbe(client, cfg, key, stdout, stderr)
	case cfg.modelsGet:
		return runModelsGet(client, key, stdout, stderr)
	}
	return runLabelled(client, cfg, key, stdout, stderr)
}

// parseFlags reads the command line. A negative code means "go on".
func parseFlags(args []string, stderr io.Writer) (*config, *flag.FlagSet, rawFlags, int) {
	cfg := &config{}
	var raw rawFlags

	flags := flag.NewFlagSet("claude-spike", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { usage(stderr, flags) }

	flags.StringVar(&cfg.dir, "dir", "", "folder of labelled sample images (spam/ and ham/ subfolders, or spam_ and ham_ file prefixes)")
	flags.StringVar(&cfg.model, "model", claude.DefaultModel, "Claude model id")
	flags.StringVar(&cfg.endpoint, "endpoint", claude.DefaultEndpoint, "API host")
	flags.Float64Var(&cfg.bar, "bar", 0.85, "spam probability at or above which an image counts as flagged")
	flags.IntVar(&cfg.concurrency, "concurrency", 2, "requests in flight at once (1 to 8)")
	flags.IntVar(&cfg.limit, "limit", 0, "stop after this many labelled images (0 means all)")
	flags.StringVar(&cfg.csvPath, "csv", "", "write the per-image CSV to this file (default: standard output)")
	flags.StringVar(&raw.toolChoice, "tool-choice", "forced", "forced (the bot's request) or auto (the model decides whether to call the tool)")
	flags.StringVar(&raw.thinking, "thinking", "disabled", "disabled (the bot's request), adaptive, or omit to send no thinking field")
	flags.StringVar(&cfg.options.Effort, "effort", "", "output effort: low, medium, high, xhigh or max (default: send none)")
	flags.IntVar(&cfg.options.MaxTokens, "max-tokens", 0, "output token cap, 1 to 4096 (0 means the bot's 256)")
	flags.Float64Var(&cfg.priceIn, "price-in", 0.10, "dollars per million input tokens ("+pricingNote+")")
	flags.Float64Var(&cfg.priceOut, "price-out", 0.50, "dollars per million output tokens, thinking included ("+pricingNote+")")
	flags.BoolVar(&cfg.probe, "probe", false, "send one tiny request per image type and option variant and report how the API answers each")
	flags.BoolVar(&cfg.modelsGet, "models-get", false, "read the model's metadata (id, display name, token limits)")
	flags.BoolVar(&cfg.fakeKey, "fake-key", false, "with -probe and no "+envKey+": use a built-in fake key (the API then answers key refused)")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil, flags, raw, exitOK
		}
		return nil, flags, raw, exitUsage
	}
	if flags.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "claude-spike: unexpected argument %q\n", flags.Arg(0))
		return nil, flags, raw, exitUsage
	}
	return cfg, flags, raw, -1
}

// validEndpoint returns a message unless the endpoint is an https URL, or an http URL for a
// loopback host (for a local test server). The real API key travels in a header to this host, so
// a plain-http or hostless URL is refused before anything is sent.
func validEndpoint(endpoint string) string {
	const message = "-endpoint must be an https URL, or an http URL for localhost, 127.0.0.1 or ::1"
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil {
		return message
	}
	switch u.Scheme {
	case "https":
		return ""
	case "http":
		switch u.Hostname() {
		case "localhost", "127.0.0.1", "::1":
			return ""
		}
	}
	return message
}

// validate checks the parsed flags and fills cfg.options from the raw ones.
// It returns a message for the first problem, or "".
func validate(cfg *config, raw rawFlags) string {
	if !claude.ValidModel(cfg.model) {
		return "-model must be 1 to 64 letters, digits, dots, underscores or hyphens, starting with a letter or digit"
	}
	if msg := validEndpoint(cfg.endpoint); msg != "" {
		return msg
	}
	if cfg.bar <= 0 || cfg.bar > 1 {
		return "-bar must be above 0 and at most 1"
	}
	if cfg.concurrency < 1 || cfg.concurrency > maxConcurrency {
		return fmt.Sprintf("-concurrency must be 1 to %d", maxConcurrency)
	}
	if cfg.limit < 0 {
		return "-limit must not be negative"
	}
	if cfg.priceIn < 0 || cfg.priceOut < 0 {
		return "-price-in and -price-out must not be negative"
	}
	if cfg.probe && cfg.modelsGet {
		return "-probe and -models-get are separate modes"
	}
	if cfg.options.MaxTokens < 0 || cfg.options.MaxTokens > maxTokensLimit {
		return fmt.Sprintf("-max-tokens must be 1 to %d, or 0 for the default", maxTokensLimit)
	}

	switch raw.toolChoice {
	case "forced":
		cfg.options.ToolChoice = ""
	case "auto":
		cfg.options.ToolChoice = "auto"
	default:
		return "-tool-choice must be forced or auto"
	}
	switch raw.thinking {
	case "disabled":
		cfg.options.Thinking = ""
	case "adaptive", "omit":
		cfg.options.Thinking = raw.thinking
	default:
		return "-thinking must be disabled, adaptive or omit"
	}
	switch cfg.options.Effort {
	case "", "low", "medium", "high":
	case "xhigh", "max":
		if raw.thinking == "disabled" {
			return "-effort " + cfg.options.Effort + " needs -thinking adaptive or omit: the API answers 400 to it with thinking disabled"
		}
	default:
		return "-effort must be low, medium, high, xhigh or max"
	}

	if !cfg.probe && !cfg.modelsGet && cfg.dir == "" {
		return "-dir is required for a labelled run"
	}
	return ""
}

// usage prints the three modes, the key rule and the data warning.
func usage(w io.Writer, flags *flag.FlagSet) {
	_, _ = fmt.Fprintf(w, `claude-spike measures Claude's spam verdicts on your own labelled images.

The API key is read from the %[1]s environment variable only. It is never a
flag and is never printed. Without it the tool sends nothing and exits 2.

WARNING: every image in a run is sent to Anthropic and billed to the account of
that key. Use samples you are happy to share.

Modes:
  claude-spike -dir <folder>     labelled run: send each image in spam/ and ham/
                                 (or spam_* and ham_* files), then print a CSV row
                                 per image and a summary of accuracy, precision,
                                 recall, a sweep of bars, latency, tokens and cost.
  claude-spike -probe            send one tiny request per image type and option
                                 variant and report how the API answers each.
                                 With -fake-key and no key it uses a fake key; the
                                 API then checks the key first, so only a real key
                                 tests the request shape.
  claude-spike -models-get       read the model's metadata.

Flags:
`, envKey)
	flags.PrintDefaults()
}

// sample is one labelled image found in the folder.
type sample struct {
	rel   string
	path  string
	label string
}

// skipped counts the files the scan did not use.
type skipped struct {
	unlabelled, tooLarge, wrongType, unreadable int
}

func (s skipped) String() string {
	return fmt.Sprintf("%d without a label or not jpg/jpeg/png/webp, %d over %d bytes, %d not really an image, %d unreadable",
		s.unlabelled, s.tooLarge, claude.MaxImageBytes, s.wrongType, s.unreadable)
}

// sniffedMIME returns the image type of a file from its first bytes, never
// from its name, or "" when it is not a jpeg, png or webp.
func sniffedMIME(path string) (string, error) {
	f, err := os.Open(path) // #nosec G304 -- the owner names the folder
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	head := make([]byte, sniffBytes)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", err
	}
	mime := http.DetectContentType(head[:n])
	switch mime {
	case "image/jpeg", "image/png", "image/webp":
		return mime, nil
	}
	return "", nil
}

// scan lists the labelled images under dir in path order, up to limit (0 means
// all), and counts what it skipped.
func scan(dir string, limit int) ([]sample, skipped, error) {
	var found []sample
	var skip skipped

	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if limit > 0 && len(found) >= limit {
			return filepath.SkipAll
		}
		if !d.Type().IsRegular() {
			skip.unlabelled++
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		label, ok := labelOf(rel)
		if !ok {
			skip.unlabelled++
			return nil
		}
		info, err := d.Info()
		if err != nil {
			skip.unreadable++
			return nil
		}
		if info.Size() > claude.MaxImageBytes {
			skip.tooLarge++
			return nil
		}
		mime, err := sniffedMIME(p)
		if err != nil {
			skip.unreadable++
			return nil
		}
		if mime == "" {
			skip.wrongType++
			return nil
		}
		found = append(found, sample{rel: rel, path: p, label: label})
		return nil
	})
	return found, skip, err
}

// classifyOne sends one sample and turns the answer into a result. The
// finish text carries a status and error type at most, never a server message.
func classifyOne(client *claude.Client, s sample, key string) result {
	r := result{File: s.rel, Label: s.label}

	data, err := os.ReadFile(s.path) // #nosec G304 -- the owner names the folder
	if err != nil {
		r.Failed, r.Finish = true, "failed unreadable file"
		return r
	}
	mime := http.DetectContentType(data)

	started := time.Now()
	verdict, err := client.Classify(context.Background(), claude.Image{MIMEType: mime, Data: data})
	r.Duration = time.Since(started)

	if err != nil {
		var httpErr *claude.HTTPError
		switch {
		case errors.Is(err, claude.ErrBlocked):
			r.Blocked, r.Finish = true, "blocked"
		case errors.As(err, &httpErr):
			r.Failed = true
			r.Finish = strings.TrimSpace(fmt.Sprintf("failed HTTP %d %s", httpErr.Status, httpErr.Type))
		case errors.Is(err, claude.ErrNoVerdict):
			r.Failed, r.Finish = true, "failed no verdict"
		case errors.Is(err, claude.ErrUnsupportedImage):
			r.Failed, r.Finish = true, "failed unsupported image"
		default:
			r.Failed, r.Finish = true, "failed request error"
		}
		r.Finish = strings.ReplaceAll(r.Finish, key, "[REDACTED]")
		return r
	}

	r.Probability = verdict.SpamProbability
	r.Finish = "ok"
	r.InputTokens = verdict.InputTokens
	r.OutputTokens = verdict.OutputTokens
	return r
}

// runLabelled is the default mode: scan, classify, then write the CSV and the
// summary.
func runLabelled(client *claude.Client, cfg *config, key string, stdout, stderr io.Writer) int {
	samples, skip, err := scan(cfg.dir, cfg.limit)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "claude-spike: cannot read %s: %v\n", cfg.dir, err)
		return exitFailure
	}
	if len(samples) == 0 {
		_, _ = fmt.Fprintf(stderr, "claude-spike: no labelled images in %s (skipped: %s); nothing was sent.\n", cfg.dir, skip)
		return exitFailure
	}
	_, _ = fmt.Fprintf(stderr, "Sending %d images to Anthropic (model %s, %d at a time). Skipped: %s.\n",
		len(samples), cfg.model, cfg.concurrency, skip)

	results := make([]result, len(samples))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < cfg.concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results[i] = classifyOne(client, samples[i], key)
			}
		}()
	}
	for i := range samples {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	if code := writeOutputs(cfg, results, stdout, stderr); code != exitOK {
		return code
	}

	failed := 0
	for _, r := range results {
		if r.Failed {
			failed++
		}
	}
	if len(results) > 0 && failed == len(results) {
		_, _ = fmt.Fprintln(stderr, "Every request failed. Run -probe and -models-get first to check the key, the model id and the request shape.")
		return exitFailure
	}
	return exitOK
}

// writeOutputs writes the CSV to its file or to stdout, then the summary.
func writeOutputs(cfg *config, results []result, stdout, stderr io.Writer) int {
	csvOut := stdout
	if cfg.csvPath != "" {
		f, err := os.Create(cfg.csvPath) // #nosec G304 -- the owner names the file
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "claude-spike: cannot write %s: %v\n", cfg.csvPath, err)
			return exitFailure
		}
		defer func() { _ = f.Close() }()
		csvOut = f
	}
	if err := writeCSV(csvOut, results); err != nil {
		_, _ = fmt.Fprintf(stderr, "claude-spike: cannot write the CSV: %v\n", err)
		return exitFailure
	}
	if cfg.csvPath == "" {
		_, _ = fmt.Fprintln(stdout)
	}

	writeSummary(stdout, summarize(results, cfg.bar), sweep(results, defaultBars()), totalsOf(results, cfg.priceIn, cfg.priceOut))
	return exitOK
}

// fakeAPIKey is the obviously fake key -probe uses with -fake-key.
const fakeAPIKey = "spike-fake-key-not-a-real-key"

// runProbe sends one tiny request per probe variant and prints how the API
// answered each. It exits 1 when the API rejected a field, because that means
// a request spelling in alita/utils/claude needs a fix.
func runProbe(client *claude.Client, cfg *config, key string, stdout, stderr io.Writer) int {
	variants := probeVariants()
	if key == fakeAPIKey {
		_, _ = fmt.Fprintln(stderr, "Probing with the built-in fake key: no image of yours is sent, and the API should answer 'key refused' to every variant.")
	} else {
		_, _ = fmt.Fprintf(stderr, "Probing with your key: %d tiny 1x1 images go to Anthropic and may be billed.\n", len(variants))
	}

	rejected := 0
	_, _ = fmt.Fprintf(stdout, "Probe of model %s\n", cfg.model)
	for _, v := range variants {
		probe := *client
		probe.Options = v.Options
		verdict, err := probe.Classify(context.Background(), v.Image)
		line := strings.ReplaceAll(probeVerdict(err, verdict), key, "[REDACTED]")
		if strings.HasPrefix(line, "field rejected") {
			rejected++
		}
		_, _ = fmt.Fprintf(stdout, "  %-28s %s\n", v.Name, line)
	}
	_, _ = fmt.Fprintf(stdout, "%d of %d variants had a field the API rejected.\n", rejected, len(variants))
	if rejected > 0 {
		return exitFailure
	}
	return exitOK
}

// runModelsGet prints the configured model's metadata.
func runModelsGet(client *claude.Client, key string, stdout, stderr io.Writer) int {
	info, err := client.GetModel(context.Background())
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "claude-spike: models lookup failed: %s\n", strings.ReplaceAll(err.Error(), key, "[REDACTED]"))
		return exitFailure
	}
	p := func(format string, args ...any) { _, _ = fmt.Fprintf(stdout, format, args...) }
	p("Model id:            %s\n", info.ID)
	p("Display name:        %s\n", info.DisplayName)
	p("Max input tokens:    %d\n", info.MaxInputTokens)
	p("Max output tokens:   %d\n", info.MaxTokens)
	return exitOK
}
