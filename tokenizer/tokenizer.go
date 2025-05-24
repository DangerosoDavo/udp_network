package tokenizer

import (
	"encoding/binary"
	"sort"
	"strings"
)

type Token uint16

type Tokenizer struct {
	vocab      map[string]Token
	reverseMap map[Token]string
	mergeRules map[string]Token
	nextToken  Token
}

func NewTokenizer() *Tokenizer {
	t := &Tokenizer{
		vocab:      make(map[string]Token),
		reverseMap: make(map[Token]string),
		mergeRules: make(map[string]Token),
		nextToken:  256, // Start after ASCII
	}
	
	// Initialize with ASCII characters
	for i := 0; i < 256; i++ {
		char := string([]byte{byte(i)})
		t.vocab[char] = Token(i)
		t.reverseMap[Token(i)] = char
	}
	
	// Add common bigrams and words
	t.addCommonTokens()
	
	return t
}

func (t *Tokenizer) addCommonTokens() {
	// Common English bigrams
	bigrams := []string{
		"th", "he", "in", "er", "an", "ed", "nd", "to", "en", "ou",
		"ea", "ng", "al", "ar", "or", "as", "at", "es", "on", "re",
	}
	
	// Common words and subwords
	words := []string{
		"the", "and", "for", "are", "but", "not", "you", "with",
		"have", "this", "from", "they", "will", "more", "when",
		"tion", "ing", "ment", "ness", "ful", "less", "able",
	}
	
	for _, token := range bigrams {
		t.addToken(token)
	}
	
	for _, token := range words {
		t.addToken(token)
	}
}

func (t *Tokenizer) addToken(text string) Token {
	if tok, exists := t.vocab[text]; exists {
		return tok
	}
	
	token := t.nextToken
	t.vocab[text] = token
	t.reverseMap[token] = text
	t.nextToken++
	
	return token
}

func (t *Tokenizer) Encode(text string) []Token {
	// Greedy tokenization with longest match first
	var tokens []Token
	i := 0
	
	for i < len(text) {
		// Try to find the longest matching token
		maxLen := 0
		var bestToken Token
		found := false
		
		// Look for matches up to 20 characters
		for length := min(20, len(text)-i); length > 0; length-- {
			substr := text[i : i+length]
			if token, exists := t.vocab[substr]; exists {
				maxLen = length
				bestToken = token
				found = true
				break
			}
		}
		
		if found {
			tokens = append(tokens, bestToken)
			i += maxLen
		} else {
			// Fall back to single character
			tokens = append(tokens, Token(text[i]))
			i++
		}
	}
	
	return tokens
}

func (t *Tokenizer) Decode(tokens []Token) string {
	var builder strings.Builder
	
	for _, token := range tokens {
		if text, exists := t.reverseMap[token]; exists {
			builder.WriteString(text)
		} else if token < 256 {
			builder.WriteByte(byte(token))
		}
	}
	
	return builder.String()
}

// Train learns new tokens from a corpus using BPE-style algorithm
func (t *Tokenizer) Train(corpus []string, numMerges int) {
	// Count pair frequencies
	for i := 0; i < numMerges; i++ {
		pairCounts := make(map[string]int)
		
		for _, text := range corpus {
			tokens := t.Encode(text)
			for j := 0; j < len(tokens)-1; j++ {
				pair := t.reverseMap[tokens[j]] + t.reverseMap[tokens[j+1]]
				pairCounts[pair]++
			}
		}
		
		// Find most frequent pair
		maxCount := 0
		var bestPair string
		
		for pair, count := range pairCounts {
			if count > maxCount {
				maxCount = count
				bestPair = pair
			}
		}
		
		if maxCount < 2 {
			break // No more useful merges
		}
		
		// Add new token
		t.addToken(bestPair)
	}
}

// Compression ratio helpers
func (t *Tokenizer) CompressionRatio(text string) float64 {
	tokens := t.Encode(text)
	tokenBytes := len(tokens) * 2 // 2 bytes per token
	originalBytes := len(text)
	
	return float64(originalBytes) / float64(tokenBytes)
}

// Binary encoding/decoding for network transmission
func (t *Tokenizer) EncodeTokensToBinary(tokens []Token) []byte {
	buf := make([]byte, len(tokens)*2)
	
	for i, token := range tokens {
		binary.BigEndian.PutUint16(buf[i*2:], uint16(token))
	}
	
	return buf
}

func (t *Tokenizer) DecodeTokensFromBinary(data []byte) []Token {
	tokens := make([]Token, len(data)/2)
	
	for i := 0; i < len(tokens); i++ {
		tokens[i] = Token(binary.BigEndian.Uint16(data[i*2:]))
	}
	
	return tokens
}

// Export/Import vocabulary for sharing between clients
func (t *Tokenizer) ExportVocab() map[string]uint16 {
	export := make(map[string]uint16)
	
	for text, token := range t.vocab {
		if token >= 256 { // Only export learned tokens
			export[text] = uint16(token)
		}
	}
	
	return export
}

func (t *Tokenizer) ImportVocab(vocab map[string]uint16) {
	// Sort by token ID to ensure consistent import
	type pair struct {
		text  string
		token uint16
	}
	
	var pairs []pair
	for text, token := range vocab {
		pairs = append(pairs, pair{text, token})
	}
	
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].token < pairs[j].token
	})
	
	for _, p := range pairs {
		t.vocab[p.text] = Token(p.token)
		t.reverseMap[Token(p.token)] = p.text
		if Token(p.token) >= t.nextToken {
			t.nextToken = Token(p.token) + 1
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}