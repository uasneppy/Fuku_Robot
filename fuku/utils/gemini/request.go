package gemini

// safetyThreshold is the threshold of the four classic categories. The source
// material is black comedy that mentions cannibalism, so a stricter threshold
// would refuse lore questions; the persona rules and the bot's output filter do
// the rest.
const safetyThreshold = "BLOCK_ONLY_HIGH"

var safetyCategories = []string{
	"HARM_CATEGORY_HARASSMENT",
	"HARM_CATEGORY_HATE_SPEECH",
	"HARM_CATEGORY_SEXUALLY_EXPLICIT",
	"HARM_CATEGORY_DANGEROUS_CONTENT",
}

// Request is one conversation turn. Static is the persona and archive and must
// be the same bytes for every chat, so the implicit cache can hit; Dynamic is
// the short per-chat block after it; User is the one user message.
type Request struct {
	Static    string
	Dynamic   string
	User      string
	MaxTokens int
}

type part struct {
	Text string `json:"text"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}

type thinkingConfig struct {
	ThinkingLevel string `json:"thinkingLevel"`
}

type generationConfig struct {
	MaxOutputTokens int            `json:"maxOutputTokens"`
	ThinkingConfig  thinkingConfig `json:"thinkingConfig"`
}

type safetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

// body is the generateContent request in camelCase. It carries no
// temperature, topP, topK, candidateCount or thinkingBudget: Gemini 4 ignores
// or rejects them.
type body struct {
	SystemInstruction content          `json:"systemInstruction"`
	Contents          []content        `json:"contents"`
	GenerationConfig  generationConfig `json:"generationConfig"`
	SafetySettings    []safetySetting  `json:"safetySettings"`
}

// buildBody assembles the request: the static text first and the dynamic text
// second, as two parts of the system instruction in that order, so the prefix
// is the same for every chat.
func buildBody(in Request, opts Options) *body {
	level := opts.ThinkingLevel
	if level == "" {
		level = DefaultThinkingLevel
	}
	settings := make([]safetySetting, 0, len(safetyCategories))
	for _, category := range safetyCategories {
		settings = append(settings, safetySetting{Category: category, Threshold: safetyThreshold})
	}
	return &body{
		SystemInstruction: content{Parts: []part{{Text: in.Static}, {Text: in.Dynamic}}},
		Contents:          []content{{Role: "user", Parts: []part{{Text: in.User}}}},
		GenerationConfig: generationConfig{
			MaxOutputTokens: in.MaxTokens,
			ThinkingConfig:  thinkingConfig{ThinkingLevel: level},
		},
		SafetySettings: settings,
	}
}
