package tokenizer

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
	"sync"
)

type Token uint16
type tokenEntry struct {
	text  string
	token Token
}

// Tokenizer is safe for concurrent use. Vocabulary changes must be exchanged
// explicitly between peers; training one peer alone changes its wire dictionary.
type Tokenizer struct {
	mu         sync.RWMutex
	vocab      map[string]Token
	reverseMap map[Token]string
	prefix     [256][]tokenEntry
	nextToken  uint32
}

func NewTokenizer() *Tokenizer {
	t := &Tokenizer{vocab: make(map[string]Token, 320), reverseMap: make(map[Token]string, 320), nextToken: 256}
	for i := 0; i < 256; i++ {
		text := string([]byte{byte(i)})
		t.vocab[text] = Token(i)
		t.reverseMap[Token(i)] = text
	}
	for _, text := range []string{"th", "he", "in", "er", "an", "ed", "nd", "to", "en", "ou", "ea", "ng", "al", "ar", "or", "as", "at", "es", "on", "re", "the", "and", "for", "are", "but", "not", "you", "with", "have", "this", "from", "they", "will", "more", "when", "tion", "ing", "ment", "ness", "ful", "less", "able"} {
		t.addToken(text)
	}
	return t
}

// Caller holds mu exclusively, or is still constructing the tokenizer.
func (t *Tokenizer) addToken(text string) Token {
	if token, ok := t.vocab[text]; ok {
		return token
	}
	if text == "" || t.nextToken > 65535 {
		return 0
	}
	token := Token(t.nextToken)
	t.nextToken++
	t.vocab[text] = token
	t.reverseMap[token] = text
	if len(text) > 1 {
		entries := append(t.prefix[text[0]], tokenEntry{text, token})
		sort.Slice(entries, func(i, j int) bool {
			if len(entries[i].text) == len(entries[j].text) {
				return entries[i].text < entries[j].text
			}
			return len(entries[i].text) > len(entries[j].text)
		})
		t.prefix[text[0]] = entries
	}
	return token
}

func (t *Tokenizer) Encode(text string) []Token {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.encode(text)
}
func (t *Tokenizer) encode(text string) []Token {
	if text == "" {
		return nil
	}
	tokens := make([]Token, 0, len(text))
	for index := 0; index < len(text); {
		matched := false
		for _, entry := range t.prefix[text[index]] {
			if strings.HasPrefix(text[index:], entry.text) {
				tokens = append(tokens, entry.token)
				index += len(entry.text)
				matched = true
				break
			}
		}
		if !matched {
			tokens = append(tokens, Token(text[index]))
			index++
		}
	}
	return tokens
}
func (t *Tokenizer) Decode(tokens []Token) string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var builder strings.Builder
	builder.Grow(len(tokens))
	for _, token := range tokens {
		if token < 256 {
			builder.WriteByte(byte(token))
		} else {
			builder.WriteString(t.reverseMap[token])
		}
	}
	return builder.String()
}

// DecodeChecked rejects unknown dictionary IDs instead of silently dropping data.
func (t *Tokenizer) DecodeChecked(tokens []Token) (string, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var builder strings.Builder
	builder.Grow(len(tokens))
	for _, token := range tokens {
		if token < 256 {
			builder.WriteByte(byte(token))
			continue
		}
		text, ok := t.reverseMap[token]
		if !ok {
			return "", fmt.Errorf("unknown token ID %d", token)
		}
		builder.WriteString(text)
	}
	return builder.String(), nil
}

func (t *Tokenizer) Train(corpus []string, merges int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i := 0; i < merges && t.nextToken <= 65535; i++ {
		counts := make(map[string]int)
		for _, text := range corpus {
			tokens := t.encode(text)
			for j := 0; j+1 < len(tokens); j++ {
				pair := t.reverseMap[tokens[j]] + t.reverseMap[tokens[j+1]]
				if _, exists := t.vocab[pair]; !exists {
					counts[pair]++
				}
			}
		}
		best, count := "", 1
		for pair, n := range counts {
			if n > count || (n == count && (best == "" || pair < best)) {
				best, count = pair, n
			}
		}
		if count < 2 || best == "" {
			break
		}
		t.addToken(best)
	}
}
func (t *Tokenizer) CompressionRatio(text string) float64 {
	if text == "" {
		return 1
	}
	return float64(len(text)) / float64(len(t.Encode(text))*2)
}
func (t *Tokenizer) EncodeTokensToBinary(tokens []Token) []byte {
	data := make([]byte, len(tokens)*2)
	for i, token := range tokens {
		binary.BigEndian.PutUint16(data[i*2:], uint16(token))
	}
	return data
}
func (t *Tokenizer) DecodeTokensFromBinaryChecked(data []byte) ([]Token, error) {
	if len(data)%2 != 0 {
		return nil, fmt.Errorf("odd token byte count")
	}
	tokens := make([]Token, len(data)/2)
	for i := range tokens {
		tokens[i] = Token(binary.BigEndian.Uint16(data[i*2:]))
	}
	return tokens, nil
}
func (t *Tokenizer) DecodeTokensFromBinary(data []byte) []Token {
	tokens, _ := t.DecodeTokensFromBinaryChecked(data)
	return tokens
}
func (t *Tokenizer) ExportVocab() map[string]uint16 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make(map[string]uint16, len(t.vocab)-256)
	for text, token := range t.vocab {
		if token >= 256 {
			result[text] = uint16(token)
		}
	}
	return result
}

// ImportVocabChecked rejects conflicting IDs atomically. ASCII tokens are fixed.
func (t *Tokenizer) ImportVocabChecked(vocab map[string]uint16) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	ids := make(map[uint16]string, len(vocab))
	entries := make([]tokenEntry, 0, len(vocab))
	for text, id := range vocab {
		if text == "" || id < 256 {
			return fmt.Errorf("invalid learned token")
		}
		if existing, ok := t.vocab[text]; ok && existing != Token(id) {
			return fmt.Errorf("token text already assigned")
		}
		if existing, ok := t.reverseMap[Token(id)]; ok && existing != text {
			return fmt.Errorf("token ID already assigned")
		}
		if existing, ok := ids[id]; ok && existing != text {
			return fmt.Errorf("duplicate token ID")
		}
		ids[id] = text
		entries = append(entries, tokenEntry{text, Token(id)})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].token < entries[j].token })
	for _, entry := range entries {
		if _, exists := t.vocab[entry.text]; exists {
			continue
		}
		t.vocab[entry.text] = entry.token
		t.reverseMap[entry.token] = entry.text
		if uint32(entry.token) >= t.nextToken {
			t.nextToken = uint32(entry.token) + 1
		}
		if len(entry.text) > 1 {
			t.prefix[entry.text[0]] = append(t.prefix[entry.text[0]], entry)
		}
	}
	for index := range t.prefix {
		entries := t.prefix[index]
		sort.Slice(entries, func(i, j int) bool {
			if len(entries[i].text) == len(entries[j].text) {
				return entries[i].text < entries[j].text
			}
			return len(entries[i].text) > len(entries[j].text)
		})
	}
	return nil
}

// ImportVocab preserves the legacy signature. Use ImportVocabChecked to receive errors.
func (t *Tokenizer) ImportVocab(vocab map[string]uint16) { _ = t.ImportVocabChecked(vocab) }
