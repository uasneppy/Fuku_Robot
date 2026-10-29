package main

// The -probe mode: the request variants and the reading of each answer. The
// probe repeats the research's contract check. A variant whose field name the
// API does not know is answered "Invalid JSON payload" before the key is even
// looked at; a variant the API accepts is answered "API key not valid" when the
// key is fake. So the probe needs no real key to find a misspelled field.

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"strings"

	"github.com/divkix/Alita_Robot/alita/utils/gemini"
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
	Image   gemini.Image
	Options gemini.Options
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
// variant: each structured-output field, thinking on and off, each media
// resolution, the store flag both ways and a safety threshold.
func probeVariants() []probeVariant {
	jpegImage := gemini.Image{MIMEType: "image/jpeg", Data: tinyJPEG()}
	variants := []probeVariant{
		{Name: "image jpeg", Image: jpegImage},
		{Name: "image png", Image: gemini.Image{MIMEType: "image/png", Data: tinyPNG()}},
		{Name: "image webp", Image: gemini.Image{MIMEType: "image/webp", Data: tinyWebPBytes()}},
	}
	add := func(name string, opts gemini.Options) {
		variants = append(variants, probeVariant{Name: name, Image: jpegImage, Options: opts})
	}

	for _, field := range []string{"responseJsonSchema", "responseSchema", "responseFormat"} {
		add("schema "+field, gemini.Options{SchemaField: field})
	}
	add("thinking MINIMAL", gemini.Options{ThinkingLevel: "MINIMAL"})
	add("thinking off", gemini.Options{ThinkingLevel: "off"})
	for _, res := range []string{"MEDIA_RESOLUTION_LOW", "MEDIA_RESOLUTION_MEDIUM", "MEDIA_RESOLUTION_HIGH"} {
		add("mediaResolution "+strings.TrimPrefix(res, "MEDIA_RESOLUTION_"), gemini.Options{MediaResolution: res})
	}
	on, off := true, false
	add("store true", gemini.Options{Store: &on})
	add("store false", gemini.Options{Store: &off})
	add("safety BLOCK_NONE", gemini.Options{SafetyThreshold: "BLOCK_NONE"})
	return variants
}

// probeVerdict reads the answer to one probe request. It reads only the
// error's status, code, reason and the start of its message, which the client
// has already cleared of the key.
func probeVerdict(err error, v *gemini.Verdict) string {
	if err == nil {
		if v == nil {
			return "accepted, but no verdict came back"
		}
		return fmt.Sprintf("accepted and answered (spam probability %.2f)", v.SpamProbability)
	}

	var httpErr *gemini.HTTPError
	if errors.As(err, &httpErr) {
		switch {
		case httpErr.Status == 400 && strings.HasPrefix(httpErr.Message, "Invalid JSON payload"):
			return "field rejected: " + quote(httpErr.Message)
		case httpErr.Reason == "API_KEY_INVALID":
			return "shape accepted, key refused"
		}
		text := fmt.Sprintf("rejected: HTTP %d", httpErr.Status)
		if httpErr.Code != "" {
			text += " " + httpErr.Code
		}
		if httpErr.Message != "" {
			text += ": " + quote(httpErr.Message)
		}
		return text
	}

	switch {
	case errors.Is(err, gemini.ErrBlocked):
		return "shape accepted, answer blocked by the safety filter"
	case errors.Is(err, gemini.ErrNoVerdict):
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
