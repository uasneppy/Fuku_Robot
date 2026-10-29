// Command gemini-spike is the owner's Gemini spike tool (see 07-SPIKE.md in
// the phase folder). It sends labelled sample images through the same
// alita/utils/gemini client the bot uses and prints accuracy, latency, tokens
// and an estimated cost, so the owner can decide the Gemini constants.
//
// It reads GEMINI_API_KEY from the environment only, never from a flag, and
// never prints it. Every sample image is sent to Google during a run. The tool
// is not part of the bot and imports nothing from the repository except the
// standard-library-only gemini package, so it never triggers the database
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
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/divkix/Alita_Robot/alita/utils/gemini"
)

const (
	envKey = "GEMINI_API_KEY"

	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2

	maxConcurrency = 8
	sniffBytes     = 512
)

// wordPattern bounds the free-text option flags before they reach a request.
var wordPattern = regexp.MustCompile(`^[A-Za-z0-9_]{0,64}$`)

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
	options     gemini.Options
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
		_, _ = fmt.Fprintf(stderr, "gemini-spike: %s\n", msg)
		return exitUsage
	}

	client := &gemini.Client{
		Endpoint: cfg.endpoint,
		APIKey:   key,
		Model:    cfg.model,
		HTTP: &http.Client{
			Transport: transport,
			Timeout:   gemini.Timeout,
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

// rawFlags are the option flags that need a check after parsing.
type rawFlags struct {
	temperature string
	store       string
	thinking    string
}

// parseFlags reads the command line. A negative code means "go on".
func parseFlags(args []string, stderr io.Writer) (*config, *flag.FlagSet, rawFlags, int) {
	cfg := &config{}
	var raw rawFlags

	flags := flag.NewFlagSet("gemini-spike", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { usage(stderr, flags) }

	flags.StringVar(&cfg.dir, "dir", "", "folder of labelled sample images (spam/ and ham/ subfolders, or spam_ and ham_ file prefixes)")
	flags.StringVar(&cfg.model, "model", gemini.DefaultModel, "Gemini model id")
	flags.StringVar(&cfg.endpoint, "endpoint", gemini.DefaultEndpoint, "API host")
	flags.Float64Var(&cfg.bar, "bar", 0.85, "spam probability at or above which an image counts as flagged")
	flags.IntVar(&cfg.concurrency, "concurrency", 2, "requests in flight at once (1 to 8)")
	flags.IntVar(&cfg.limit, "limit", 0, "stop after this many labelled images (0 means all)")
	flags.StringVar(&cfg.csvPath, "csv", "", "write the per-image CSV to this file (default: standard output)")
	flags.StringVar(&cfg.options.MediaResolution, "media-res", "", "mediaResolution value, for example MEDIA_RESOLUTION_LOW (default MEDIA_RESOLUTION_MEDIUM)")
	flags.StringVar(&cfg.options.SchemaField, "schema-field", "", "structured-output field: responseJsonSchema (default), responseSchema or responseFormat")
	flags.StringVar(&raw.thinking, "thinking", "", "thinking level, or off to send none (default: MINIMAL on gemini-3 models)")
	flags.StringVar(&raw.temperature, "temperature", "0", "0 to send temperature 0, or default to leave it out")
	flags.StringVar(&cfg.options.SafetyThreshold, "safety", "", "safety threshold for the four adjustable categories, for example BLOCK_NONE (default: send none)")
	flags.StringVar(&raw.store, "store", "unset", "unset, true or false: the request's store flag")
	flags.Float64Var(&cfg.priceIn, "price-in", 0.30, "dollars per million input tokens ("+pricingNote+")")
	flags.Float64Var(&cfg.priceOut, "price-out", 2.50, "dollars per million output tokens, thoughts included ("+pricingNote+")")
	flags.BoolVar(&cfg.probe, "probe", false, "send one tiny request per image type and option variant and report which fields the API accepts")
	flags.BoolVar(&cfg.modelsGet, "models-get", false, "read the model's metadata (name, token limits, generation methods)")
	flags.BoolVar(&cfg.fakeKey, "fake-key", false, "with -probe and no "+envKey+": use a built-in fake key (the API then answers key refused)")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil, flags, raw, exitOK
		}
		return nil, flags, raw, exitUsage
	}
	if flags.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "gemini-spike: unexpected argument %q\n", flags.Arg(0))
		return nil, flags, raw, exitUsage
	}
	return cfg, flags, raw, -1
}

// validate checks the parsed flags and fills cfg.options from the raw ones.
// It returns a message for the first problem, or "".
func validate(cfg *config, raw rawFlags) string {
	if !gemini.ValidModel(cfg.model) {
		return "-model must be 1 to 64 letters, digits, dots, underscores or hyphens"
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
	switch cfg.options.SchemaField {
	case "", "responseJsonSchema", "responseSchema", "responseFormat":
	default:
		return "-schema-field must be responseJsonSchema, responseSchema or responseFormat"
	}
	for name, value := range map[string]string{
		"-media-res": cfg.options.MediaResolution,
		"-safety":    cfg.options.SafetyThreshold,
		"-thinking":  raw.thinking,
	} {
		if !wordPattern.MatchString(value) {
			return name + " must be letters, digits and underscores only"
		}
	}
	cfg.options.ThinkingLevel = raw.thinking

	switch raw.temperature {
	case "0":
	case "default":
		cfg.options.OmitTemperature = true
	default:
		return "-temperature must be 0 or default"
	}
	switch raw.store {
	case "unset":
	case "true", "false":
		value := raw.store == "true"
		cfg.options.Store = &value
	default:
		return "-store must be unset, true or false"
	}
	if !cfg.probe && !cfg.modelsGet && cfg.dir == "" {
		return "-dir is required for a labelled run"
	}
	return ""
}

// usage prints the three modes, the key rule and the data warning.
func usage(w io.Writer, flags *flag.FlagSet) {
	_, _ = fmt.Fprintf(w, `gemini-spike measures Gemini's spam verdicts on your own labelled images.

The API key is read from the %[1]s environment variable only. It is never a
flag and is never printed. Without it the tool sends nothing and exits 2.

WARNING: every image in a run is sent to Google. Use a paid-tier key (see
07-SPIKE.md) and samples you are happy to share.

Modes:
  gemini-spike -dir <folder>     labelled run: send each image in spam/ and ham/
                                 (or spam_* and ham_* files), then print a CSV row
                                 per image and a summary of accuracy, precision,
                                 recall, a sweep of bars, latency, tokens and cost.
  gemini-spike -probe            send one tiny request per image type and option
                                 variant and report which fields the API accepts.
                                 With -fake-key and no key it uses a fake key.
  gemini-spike -models-get       read the model's metadata.

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
		s.unlabelled, s.tooLarge, gemini.MaxImageBytes, s.wrongType, s.unreadable)
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
		if info.Size() > gemini.MaxImageBytes {
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
// finish text carries a status and code at most, never a server message.
func classifyOne(client *gemini.Client, s sample, key string) result {
	r := result{File: s.rel, Label: s.label}

	data, err := os.ReadFile(s.path) // #nosec G304 -- the owner names the folder
	if err != nil {
		r.Failed, r.Finish = true, "failed unreadable file"
		return r
	}
	mime := http.DetectContentType(data)

	started := time.Now()
	verdict, err := client.Classify(context.Background(), gemini.Image{MIMEType: mime, Data: data})
	r.Duration = time.Since(started)

	if err != nil {
		switch {
		case errors.Is(err, gemini.ErrBlocked):
			r.Blocked, r.Finish = true, "blocked"
		case isHTTPError(err):
			var httpErr *gemini.HTTPError
			_ = errors.As(err, &httpErr)
			r.Failed = true
			r.Finish = strings.TrimSpace(fmt.Sprintf("failed HTTP %d %s", httpErr.Status, httpErr.Code))
		case errors.Is(err, gemini.ErrNoVerdict):
			r.Failed, r.Finish = true, "failed no verdict"
		case errors.Is(err, gemini.ErrUnsupportedImage):
			r.Failed, r.Finish = true, "failed unsupported image"
		default:
			r.Failed, r.Finish = true, "failed request error"
		}
		r.Finish = strings.ReplaceAll(r.Finish, key, "[REDACTED]")
		return r
	}

	r.Probability = verdict.SpamProbability
	r.Category = verdict.Category
	r.Finish = "ok"
	r.PromptTokens = verdict.PromptTokens
	r.OutputTokens = verdict.OutputTokens
	r.ThoughtTokens = verdict.ThoughtTokens
	return r
}

func isHTTPError(err error) bool {
	var httpErr *gemini.HTTPError
	return errors.As(err, &httpErr)
}

// runLabelled is the default mode: scan, classify, then write the CSV and the
// summary.
func runLabelled(client *gemini.Client, cfg *config, key string, stdout, stderr io.Writer) int {
	samples, skip, err := scan(cfg.dir, cfg.limit)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "gemini-spike: cannot read %s: %v\n", cfg.dir, err)
		return exitFailure
	}
	if len(samples) == 0 {
		_, _ = fmt.Fprintf(stderr, "gemini-spike: no labelled images in %s (skipped: %s); nothing was sent.\n", cfg.dir, skip)
		return exitFailure
	}
	_, _ = fmt.Fprintf(stderr, "Sending %d images to Google (model %s, %d at a time). Skipped: %s.\n",
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
	if failed == len(results) {
		_, _ = fmt.Fprintln(stderr, "Every request failed. Run -probe and -models-get first to check the key, the model id and the request shape.")
	}
	return exitOK
}

// writeOutputs writes the CSV to its file or to stdout, then the summary.
func writeOutputs(cfg *config, results []result, stdout, stderr io.Writer) int {
	csvOut := stdout
	if cfg.csvPath != "" {
		f, err := os.Create(cfg.csvPath) // #nosec G304 -- the owner names the file
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "gemini-spike: cannot write %s: %v\n", cfg.csvPath, err)
			return exitFailure
		}
		defer func() { _ = f.Close() }()
		csvOut = f
	}
	if err := writeCSV(csvOut, results); err != nil {
		_, _ = fmt.Fprintf(stderr, "gemini-spike: cannot write the CSV: %v\n", err)
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
// answered each. It exits 1 when the API rejected a field name, because that
// means a request spelling in alita/utils/gemini needs a fix.
func runProbe(client *gemini.Client, cfg *config, key string, stdout, stderr io.Writer) int {
	variants := probeVariants()
	if key == fakeAPIKey {
		_, _ = fmt.Fprintln(stderr, "Probing with the built-in fake key: no image of yours is sent, and the API should answer 'key refused' to every accepted shape.")
	} else {
		_, _ = fmt.Fprintf(stderr, "Probing with your key: %d tiny 1x1 images go to Google and may be billed.\n", len(variants))
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
	_, _ = fmt.Fprintf(stdout, "%d of %d variants had a field name the API does not know.\n", rejected, len(variants))
	if rejected > 0 {
		return exitFailure
	}
	return exitOK
}

// runModelsGet prints the configured model's metadata.
func runModelsGet(client *gemini.Client, key string, stdout, stderr io.Writer) int {
	info, err := client.GetModel(context.Background())
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "gemini-spike: models.get failed: %s\n", strings.ReplaceAll(err.Error(), key, "[REDACTED]"))
		return exitFailure
	}
	generate := "no"
	for _, method := range info.SupportedGenerationMethods {
		if method == "generateContent" {
			generate = "yes"
		}
	}
	p := func(format string, args ...any) { _, _ = fmt.Fprintf(stdout, format, args...) }
	p("Name:                %s\n", info.Name)
	p("Display name:        %s\n", info.DisplayName)
	p("Input token limit:   %d\n", info.InputTokenLimit)
	p("Output token limit:  %d\n", info.OutputTokenLimit)
	p("Generation methods:  %s\n", strings.Join(info.SupportedGenerationMethods, ", "))
	p("Takes generateContent: %s\n", generate)
	return exitOK
}
