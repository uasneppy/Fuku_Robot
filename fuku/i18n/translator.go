package i18n

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"

	log "github.com/sirupsen/logrus"
)

var (
	// interpolationToken matches a {name} placeholder, an escaped %% or a legacy verb.
	interpolationToken = regexp.MustCompile(`\{[^}]+\}|%%|%[sdvfbtoxX]`)
)

func (t *Translator) GetString(key string, params ...TranslationParams) (string, error) {
	if t.manager == nil {
		return "", NewI18nError("get_string", t.langCode, key, "manager not initialized", ErrManagerNotInit)
	}

	result := t.lookupString(key)

	if result == "" || result == "<nil>" {
		if t.langCode != t.manager.defaultLang {
			defaultTranslator, err := t.manager.GetTranslator(t.manager.defaultLang)
			if err != nil {
				return "", NewI18nError("get_string", t.langCode, key, "fallback failed", err)
			}
			if defaultTranslator.langCode == t.langCode {
				return "", NewI18nError("get_string", t.langCode, key, "recursive fallback detected", ErrRecursiveFallback)
			}
			return defaultTranslator.GetString(key, params...)
		}
		return "", NewI18nError("get_string", t.langCode, key, "translation not found", ErrKeyNotFound)
	}

	if len(params) > 0 {
		var err error
		result, err = t.interpolateParams(result, params[0])
		if err != nil {
			return result, NewI18nError("get_string", t.langCode, key, "parameter interpolation failed", err)
		}
	}

	return result, nil
}

func (t *Translator) GetStringSlice(key string) ([]string, error) {
	if t.manager == nil {
		return nil, NewI18nError("get_string_slice", t.langCode, key, "manager not initialized", ErrManagerNotInit)
	}

	result := t.lookupStringSlice(key)

	if len(result) == 0 {
		if t.langCode != t.manager.defaultLang {
			defaultTranslator, err := t.manager.GetTranslator(t.manager.defaultLang)
			if err != nil {
				return nil, NewI18nError("get_string_slice", t.langCode, key, "fallback failed", err)
			}
			if defaultTranslator.langCode == t.langCode {
				return nil, NewI18nError("get_string_slice", t.langCode, key, "recursive fallback detected", ErrRecursiveFallback)
			}
			return defaultTranslator.GetStringSlice(key)
		}
		return nil, NewI18nError("get_string_slice", t.langCode, key, "translation not found", ErrKeyNotFound)
	}

	// The lookup may hand back storage the index or the parsed map still owns, so callers
	// get a copy they can mutate.
	return slices.Clone(result), nil
}

// interpolateParams fills the template in one pass over the raw template text: each {name}
// placeholder takes its param, and each legacy verb (%s, %d, ...) the next ordered value, with
// %% printing %. The verbs are counted on the template alone, never on a filled-in value, so a
// value that holds a verb or a {name} (text a user typed) is printed as it is and can neither
// shift nor block the template's own verbs (I4). The verbs are filled only when the template
// has no more of them than there are ordered values, as before; otherwise they stay raw.
func (t *Translator) interpolateParams(text string, params TranslationParams) (string, error) {
	if params == nil {
		return text, nil
	}

	verbs := 0
	for _, token := range interpolationToken.FindAllString(text, -1) {
		if token[0] == '%' && token != "%%" {
			verbs++
		}
	}
	var ordered []any
	if verbs > 0 {
		if values := extractOrderedValues(params); len(values) > 0 {
			if verbs <= len(values) {
				ordered = values[:verbs]
			} else {
				log.Warnf("Translation specifier count mismatch: %d specifiers, %d values for key in lang %s", verbs, len(values), t.langCode)
			}
		}
	}

	next := 0
	result := interpolationToken.ReplaceAllStringFunc(text, func(token string) string {
		switch {
		case token[0] == '{':
			if value, exists := params[token[1:len(token)-1]]; exists {
				return fmt.Sprintf("%v", value)
			}
			return token
		case ordered == nil:
			return token
		case token == "%%":
			return "%"
		default:
			value := ordered[next]
			next++
			return fmt.Sprintf(token, value)
		}
	})

	return result, nil
}

func extractOrderedValues(params TranslationParams) []any {
	if params == nil {
		return nil
	}

	var values []any

	for i := 0; i < 10; i++ {
		key := strconv.Itoa(i)
		if value, exists := params[key]; exists {
			values = append(values, value)
		} else {
			break
		}
	}

	if len(values) == 0 {
		commonKeys := []string{
			"first", "second", "third", "fourth", "fifth",
			"question", "answer",
			"number", "count", "value",
			"name", "user", "username",
			"arg1", "arg2", "arg3",
			"s", "d", "v", "f",
			"duration", "threshold", "error", "user_id", "expires_in", "raid_time", "action_time", "auto_threshold", "mode", "reason", "limit",
		}
		for _, key := range commonKeys {
			if value, exists := params[key]; exists {
				values = append(values, value)
			}
		}
	}

	return values
}

// LangCode is the language code the translator was made for.
func (t *Translator) LangCode() string {
	if t == nil {
		return ""
	}
	return t.langCode
}
