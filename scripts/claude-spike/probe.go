package main

// The -probe mode: the request variants and the reading of each answer. Each
// variant changes one thing from the bot's request, so the owner sees which
// shapes the API accepts. A fake key is answered "key refused" before the body
// is read, so only a real key tests the shape.

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"

	"github.com/divkix/Alita_Robot/alita/utils/claude"
)

// maxQuoteRunes bounds the part of a server message the probe prints.
const maxQuoteRunes = 110

// tinyWebP is a 1x1 lossless WebP. The standard library has no WebP encoder, so
// it is a literal (base64 of the file's 26 bytes).
const tinyWebP = "UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA=="

// probeVariant is one probe request: an image and the options that change the
// request from the production one.
type probeVariant struct {
	Name    string
	Image   claude.Image
	Options claude.Options
}

func tinyJPEG() []byte {
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1)), nil)
	return buf.Bytes()
}

func tinyPNG() []byte {
	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1)))
	return buf.Bytes()
}

func tinyWebPBytes() []byte {
	data, err := base64.StdEncoding.DecodeString(tinyWebP)
	if err != nil {
		return nil
	}
	return data
}

// probeVariants lists one request per image type and one per optional
// variant: automatic tool choice, adaptive thinking, thinking left out and an
// effort setting.
func probeVariants() []probeVariant {
	jpegImage := claude.Image{MIMEType: "image/jpeg", Data: tinyJPEG()}
	variants := []probeVariant{
		{Name: "image jpeg", Image: jpegImage},
		{Name: "image png", Image: claude.Image{MIMEType: "image/png", Data: tinyPNG()}},
		{Name: "image webp", Image: claude.Image{MIMEType: "image/webp", Data: tinyWebPBytes()}},
	}
	add := func(name string, opts claude.Options) {
		variants = append(variants, probeVariant{Name: name, Image: jpegImage, Options: opts})
	}

	add("tool choice auto", claude.Options{ToolChoice: "auto"})
	add("thinking adaptive", claude.Options{Thinking: "adaptive", MaxTokens: 1024})
	add("thinking omitted", claude.Options{Thinking: "omit", MaxTokens: 1024})
	add("effort low", claude.Options{Effort: "low"})
	return variants
}

// probeVerdict reads the answer to one probe request. It reads only the
// error's status, type and the start of its message, which the client has
// already cleared of the key.
func probeVerdict(err error, v *claude.Verdict) string {
	if err == nil {
		if v == nil {
			return "accepted, but no verdict came back"
		}
		return fmt.Sprintf("accepted and answered (spam probability %.2f)", v.SpamProbability)
	}

	var httpErr *claude.HTTPError
	if errors.As(err, &httpErr) {
		switch {
		case httpErr.Status == http.StatusUnauthorized:
			return "key refused"
		case httpErr.Status == http.StatusBadRequest && httpErr.Type == "invalid_request_error":
			return "field rejected: " + quote(httpErr.Message)
		}
		text := fmt.Sprintf("rejected: HTTP %d", httpErr.Status)
		if httpErr.Type != "" {
			text += " " + httpErr.Type
		}
		if httpErr.Message != "" {
			text += ": " + quote(httpErr.Message)
		}
		return text
	}

	switch {
	case errors.Is(err, claude.ErrBlocked):
		return "shape accepted, refused by the provider"
	case errors.Is(err, claude.ErrNoVerdict):
		return "shape accepted, answer was not a usable verdict"
	}
	return "request error (no HTTP answer)"
}

// quote cuts a server message to its start.
func quote(message string) string {
	runes := []rune(message)
	if len(runes) > maxQuoteRunes {
		return string(runes[:maxQuoteRunes]) + "..."
	}
	return message
}
