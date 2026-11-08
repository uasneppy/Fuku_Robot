//go:build testtools

package claude

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// namesBody is the decoded request body the server saw.
type namesBody struct {
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens"`
	System    string `json:"system"`
	Thinking  struct {
		Type string `json:"type"`
	} `json:"thinking"`
	Tools []struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Strict      bool           `json:"strict"`
		InputSchema map[string]any `json:"input_schema"`
	} `json:"tools"`
	ToolChoice struct {
		Type                   string `json:"type"`
		Name                   string `json:"name"`
		DisableParallelToolUse bool   `json:"disable_parallel_tool_use"`
	} `json:"tool_choice"`
	Messages []struct {
		Role    string `json:"role"`
		Content []struct {
			Type   string          `json:"type"`
			Text   string          `json:"text"`
			Source json.RawMessage `json:"source"`
		} `json:"content"`
	} `json:"messages"`
}

// namesSeen is the object the user block must decode to.
type namesSeen struct {
	First    string `json:"first_name"`
	Last     string `json:"last_name"`
	Username string `json:"username"`
}

// classifyNamesOnce runs ClassifyNames against a server answering with body
// and returns the verdict, the recorder and the error.
func classifyNamesOnce(t *testing.T, names Names, answer string) (*Verdict, *recorder, error) {
	t.Helper()
	server, rec := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, answer)
	})
	verdict, err := testClient(server, testModel).ClassifyNames(context.Background(), names)
	return verdict, rec, err
}

// decodeNamesBody decodes the one request a recorder holds.
func decodeNamesBody(t *testing.T, rec *recorder) namesBody {
	t.Helper()
	reqs := rec.all()
	if len(reqs) != 1 {
		t.Fatalf("requests = %d, want exactly 1", len(reqs))
	}
	var body namesBody
	if err := json.Unmarshal(reqs[0].Body, &body); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}
	return body
}

// namesSeenOf decodes the single text block of the request into the object the
// model is shown, failing on any other shape.
func namesSeenOf(t *testing.T, body namesBody) namesSeen {
	t.Helper()
	if len(body.Messages) != 1 || body.Messages[0].Role != "user" {
		t.Fatalf("messages = %+v, want exactly one user message", body.Messages)
	}
	blocks := body.Messages[0].Content
	if len(blocks) != 1 || blocks[0].Type != "text" {
		t.Fatalf("content = %+v, want exactly one text block", blocks)
	}
	var keys map[string]string
	if err := json.Unmarshal([]byte(blocks[0].Text), &keys); err != nil {
		t.Fatalf("the user text is not a JSON object of strings: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("user object keys = %v, want exactly first_name, last_name and username", keys)
	}
	for _, key := range []string{"first_name", "last_name", "username"} {
		if _, ok := keys[key]; !ok {
			t.Fatalf("user object lacks %q: %v", key, keys)
		}
	}
	return namesSeen{First: keys["first_name"], Last: keys["last_name"], Username: keys["username"]}
}

func TestClassifyNames(t *testing.T) {
	t.Run("sends the names request", func(t *testing.T) {
		names := Names{First: "Anna", Last: "", Username: "anna_k"}
		verdict, rec, err := classifyNamesOnce(t, names, toolAnswer(`{"spam_probability":0.97}`))
		if err != nil {
			t.Fatalf("ClassifyNames: %v", err)
		}
		if verdict.SpamProbability != 0.97 || verdict.InputTokens != 790 || verdict.OutputTokens != 21 {
			t.Fatalf("verdict = %+v", verdict)
		}

		req := rec.all()[0]
		if req.Method != http.MethodPost || req.Path != "/v1/messages" || req.RawQuery != "" {
			t.Fatalf("request = %s %s ?%s", req.Method, req.Path, req.RawQuery)
		}
		if req.Header.Get("x-api-key") != testKey || req.Header.Get("anthropic-version") != "2026-10-21" {
			t.Fatalf("headers = %v", req.Header)
		}

		body := decodeNamesBody(t, rec)
		if body.Model != testModel || body.MaxTokens != 128 || body.System != NamesInstruction {
			t.Fatalf("model %q max_tokens %d system %q", body.Model, body.MaxTokens, body.System)
		}
		if body.Thinking.Type != "disabled" {
			t.Fatalf("thinking = %q", body.Thinking.Type)
		}
		if len(body.Tools) != 1 || body.Tools[0].Name != ToolName || !body.Tools[0].Strict {
			t.Fatalf("tools = %+v", body.Tools)
		}
		schema := body.Tools[0].InputSchema
		props, _ := schema["properties"].(map[string]any)
		if len(props) != 1 || props["spam_probability"] == nil || schema["additionalProperties"] != false {
			t.Fatalf("schema = %v, want only spam_probability and no additional properties", schema)
		}
		if body.ToolChoice.Type != "tool" || body.ToolChoice.Name != ToolName || !body.ToolChoice.DisableParallelToolUse {
			t.Fatalf("tool_choice = %+v", body.ToolChoice)
		}
		if got := namesSeenOf(t, body); got != (namesSeen{First: "Anna", Username: "anna_k"}) {
			t.Fatalf("user object = %+v", got)
		}
		for _, block := range body.Messages[0].Content {
			if block.Type == "image" || len(block.Source) != 0 {
				t.Fatalf("a names request carries no image: %+v", block)
			}
		}

		var raw map[string]any
		if err := json.Unmarshal(req.Body, &raw); err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{"temperature", "top_p", "top_k", "output_config"} {
			if _, ok := raw[banned]; ok {
				t.Errorf("%s must not be sent", banned)
			}
		}
	})

	t.Run("the tool of a names request is described for names", func(t *testing.T) {
		_, rec, err := classifyNamesOnce(t, Names{First: "Anna"}, toolAnswer(goodAnswer))
		if err != nil {
			t.Fatal(err)
		}
		body := decodeNamesBody(t, rec)
		if body.Tools[0].Description == "Report how likely the image is spam." {
			t.Fatal("the names tool carries the image description")
		}
	})
}
