// Command chat-spike is the owner's tool for checking Xenobot-72's character
// before conversations are enabled anywhere. It reads a file of questions,
// sends each one through the same request builder (fuku/utils/chatprompt) and
// provider client (fuku/utils/claude or fuku/utils/gemini) the bot uses, and
// prints the question, the reply, the outcome, the token counts and the
// latency.
//
// It reads ANTHROPIC_API_KEY or GEMINI_API_KEY from the environment only, never
// from a flag, and never prints it. Every question in a run is sent to the
// chosen provider. The tool is not part of the bot and imports only leaf
// packages, so it never triggers the database connection that fuku/config and
// fuku/db make before main.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/utils/chatprompt"
	"github.com/uasneppy/fuku_robot/fuku/utils/claude"
	"github.com/uasneppy/fuku_robot/fuku/utils/gemini"
)

const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2

	providerAnthropic = "anthropic"
	providerGoogle    = "google"

	// defaultMaxTokens is the bot's output cap, thinking included.
	defaultMaxTokens = 2000
	maxQuestions     = 500
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr, http.DefaultTransport))
}

type config struct {
	provider    string
	questions   string
	model       string
	endpoint    string
	name        string
	title       string
	language    string
	date        string
	botUsername string
	maxTokens   int
	thinking    string
}

// reply is what both providers' answers are reduced to.
type reply struct {
	text      string
	outcome   string
	input     int
	cacheRead int
	output    int
}

type converser func(ctx context.Context, static, dynamic, user string, maxTokens int) (reply, error)

// run is main without the process. It returns the exit code. Nothing is sent
// before the key has been found and every flag has been checked.
func run(args []string, getenv func(string) string, stdout, stderr io.Writer, transport http.RoundTripper) int {
	cfg, code := parseFlags(args, stderr)
	if code >= 0 {
		return code
	}
	keyName := "ANTHROPIC_API_KEY"
	if cfg.provider == providerGoogle {
		keyName = "GEMINI_API_KEY"
	}
	key := getenv(keyName)
	if key == "" {
		_, _ = fmt.Fprintf(stderr, "%s is not set, so nothing was sent.\n", keyName)
		return exitUsage
	}
	if msg := validate(cfg); msg != "" {
		_, _ = fmt.Fprintf(stderr, "chat-spike: %s\n", msg)
		return exitUsage
	}
	questions, err := readQuestions(cfg.questions)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "chat-spike: %v\n", err)
		return exitUsage
	}

	hc := &http.Client{
		Transport: transport,
		Timeout:   claude.ConverseTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	send := newConverser(cfg, key, hc)

	static := chatprompt.Static()
	dynamic := chatprompt.Dynamic(chatprompt.Chat{
		Title: cfg.title, Language: cfg.language, Date: cfg.date, BotUsername: cfg.botUsername,
	})
	_, _ = fmt.Fprintf(stdout, "provider %s, model %s, %d questions\n\n", cfg.provider, cfg.model, len(questions))

	var sum summary
	for i, q := range questions {
		user := chatprompt.User(nil, nil, chatprompt.Turn{Name: cfg.name, Text: q})
		start := time.Now()
		r, err := send(context.Background(), static, dynamic, user, cfg.maxTokens)
		elapsed := time.Since(start)
		sum.add(r, err, elapsed)

		_, _ = fmt.Fprintf(stdout, "### %d\nQ: %s\n", i+1, q)
		if err != nil {
			_, _ = fmt.Fprintf(stdout, "ERROR: %s\nlatency=%s\n\n", scrub(err.Error(), key), elapsed.Round(time.Millisecond))
			continue
		}
		_, _ = fmt.Fprintf(stdout, "A: %s\noutcome=%s in=%d cached=%d out=%d latency=%s\n\n",
			scrub(r.text, key), r.outcome, r.input, r.cacheRead, r.output, elapsed.Round(time.Millisecond))
	}
	sum.print(stdout)
	if sum.failed == len(questions) {
		return exitFailure
	}
	return exitOK
}

func parseFlags(args []string, stderr io.Writer) (*config, int) {
	cfg := &config{}
	fs := flag.NewFlagSet("chat-spike", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.provider, "provider", providerAnthropic, "anthropic or google")
	fs.StringVar(&cfg.questions, "questions", "", "file with one question per line (# starts a comment)")
	fs.StringVar(&cfg.model, "model", "", "model id (default: claude-haiku-5-5 or gemini-4-hydrogen)")
	fs.StringVar(&cfg.endpoint, "endpoint", "", "API host (default: the provider's)")
	fs.StringVar(&cfg.name, "name", "Olena", "first name of the asking member")
	fs.StringVar(&cfg.title, "title", "Test facility", "group title")
	fs.StringVar(&cfg.language, "language", "Ukrainian", "group language name")
	fs.StringVar(&cfg.date, "date", time.Now().Format("2 January 2006"), "today's date as the bot states it")
	fs.StringVar(&cfg.botUsername, "bot-username", "", "the bot's Telegram username, without @")
	fs.IntVar(&cfg.maxTokens, "max-tokens", defaultMaxTokens, "output cap, thinking included")
	fs.StringVar(&cfg.thinking, "thinking-level", "", "Gemini only: thinkingLevel to send (default low)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil, exitOK
		}
		return nil, exitUsage
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "chat-spike: unexpected argument %q\n", fs.Arg(0))
		return nil, exitUsage
	}
	if cfg.model == "" {
		cfg.model = claude.DefaultModel
		if cfg.provider == providerGoogle {
			cfg.model = gemini.DefaultModel
		}
	}
	return cfg, -1
}

func validate(cfg *config) string {
	switch {
	case cfg.provider != providerAnthropic && cfg.provider != providerGoogle:
		return "-provider must be anthropic or google"
	case cfg.questions == "":
		return "-questions is required"
	case !claude.ValidModel(cfg.model):
		return "-model must be 1 to 64 letters, digits, dots, underscores or hyphens"
	case cfg.maxTokens < 1 || cfg.maxTokens > 16000:
		return "-max-tokens must be 1 to 16000"
	case cfg.thinking != "" && cfg.provider != providerGoogle:
		return "-thinking-level is for -provider google only"
	}
	if cfg.endpoint != "" {
		return validEndpoint(cfg.endpoint)
	}
	return ""
}

// validEndpoint refuses anything but https, except plain http to the local
// machine, because the key goes to that host in a header.
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

func readQuestions(path string) ([]string, error) {
	f, err := os.Open(path) // #nosec G304 -- the owner names the file
	if err != nil {
		return nil, errors.New("cannot open the questions file")
	}
	defer func() { _ = f.Close() }()

	var out []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
		if len(out) > maxQuestions {
			return nil, fmt.Errorf("more than %d questions", maxQuestions)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.New("cannot read the questions file")
	}
	if len(out) == 0 {
		return nil, errors.New("the questions file has no question")
	}
	return out, nil
}

func newConverser(cfg *config, key string, hc *http.Client) converser {
	if cfg.provider == providerGoogle {
		c := &gemini.Client{Endpoint: cfg.endpoint, APIKey: key, Model: cfg.model, HTTP: hc,
			Options: gemini.Options{ThinkingLevel: cfg.thinking}}
		return func(ctx context.Context, static, dynamic, user string, maxTokens int) (reply, error) {
			r, err := c.Generate(ctx, gemini.Request{Static: static, Dynamic: dynamic, User: user, MaxTokens: maxTokens})
			if err != nil {
				return reply{}, err
			}
			// promptTokenCount includes the cached tokens; report the rest as input.
			return reply{text: r.Text, outcome: outcomeName(int(r.Outcome)), input: max(r.InputTokens-r.CacheReadTokens, 0),
				cacheRead: r.CacheReadTokens, output: r.OutputTokens + r.ThoughtsTokens}, nil
		}
	}
	c := &claude.Client{Endpoint: cfg.endpoint, APIKey: key, Model: cfg.model, HTTP: hc}
	return func(ctx context.Context, static, dynamic, user string, maxTokens int) (reply, error) {
		r, err := c.Converse(ctx, claude.ConverseRequest{Static: static, Dynamic: dynamic, User: user, MaxTokens: maxTokens})
		if err != nil {
			return reply{}, err
		}
		return reply{text: r.Text, outcome: outcomeName(int(r.Outcome)), input: r.InputTokens + r.CacheCreationTokens,
			cacheRead: r.CacheReadTokens, output: r.OutputTokens}, nil
	}
}

// outcomeName names both packages' outcomes, which share their numbering.
func outcomeName(o int) string {
	switch o {
	case 0:
		return "answered"
	case 1:
		return "refused"
	case 2:
		return "truncated"
	}
	return "unknown"
}

func scrub(s, key string) string {
	if key == "" {
		return s
	}
	return strings.ReplaceAll(s, key, "[REDACTED]")
}

type summary struct {
	answered, refused, truncated, failed int
	input, cacheRead, output             int
	latency                              time.Duration
}

func (s *summary) add(r reply, err error, elapsed time.Duration) {
	s.latency += elapsed
	if err != nil {
		s.failed++
		return
	}
	switch r.outcome {
	case "answered":
		s.answered++
	case "refused":
		s.refused++
	case "truncated":
		s.truncated++
	}
	s.input += r.input
	s.cacheRead += r.cacheRead
	s.output += r.output
}

func (s *summary) print(w io.Writer) {
	total := s.answered + s.refused + s.truncated + s.failed
	mean := time.Duration(0)
	if total > 0 {
		mean = s.latency / time.Duration(total)
	}
	_, _ = fmt.Fprintf(w, "Summary: %d questions, %d answered, %d refused, %d truncated, %d failed\n",
		total, s.answered, s.refused, s.truncated, s.failed)
	_, _ = fmt.Fprintf(w, "Tokens: %d input (uncached), %d cache read, %d output; mean latency %s\n",
		s.input, s.cacheRead, s.output, mean.Round(time.Millisecond))
}
