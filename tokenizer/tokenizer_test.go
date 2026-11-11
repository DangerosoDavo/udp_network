package tokenizer

import (
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestRoundTripAndDeterministicTraining(t *testing.T) {
	for _, text := range []string{"", "the ship will return", "日本語 🚀 café", string([]byte{0, 255, 128, 1}), strings.Repeat("this", 1000)} {
		tokenizer := NewTokenizer()
		if got := tokenizer.Decode(tokenizer.Encode(text)); got != text {
			t.Fatalf("roundtrip mismatch for %q", text)
		}
	}
	a, b := NewTokenizer(), NewTokenizer()
	corpus := []string{strings.Repeat("abcd xyzw ", 100)}
	a.Train(corpus, 20)
	b.Train(corpus, 20)
	if !reflect.DeepEqual(a.ExportVocab(), b.ExportVocab()) {
		t.Fatal("training is nondeterministic")
	}
	if got := a.Decode(a.Encode(corpus[0])); got != corpus[0] {
		t.Fatal("trained roundtrip mismatch")
	}
}
func TestImportValidationAtomic(t *testing.T) {
	tokenizer := NewTokenizer()
	before := tokenizer.ExportVocab()
	if tokenizer.ImportVocabChecked(map[string]uint16{"bad": 1}) == nil {
		t.Fatal("accepted ASCII overwrite")
	}
	if !reflect.DeepEqual(before, tokenizer.ExportVocab()) {
		t.Fatal("failed import mutated vocabulary")
	}
	if tokenizer.ImportVocabChecked(map[string]uint16{"one": 600, "two": 600}) == nil {
		t.Fatal("accepted duplicate token IDs")
	}
	long := strings.Repeat("q", 24)
	if err := tokenizer.ImportVocabChecked(map[string]uint16{long: 600}); err != nil {
		t.Fatal(err)
	}
	if got := tokenizer.Encode(long); !reflect.DeepEqual(got, []Token{600}) {
		t.Fatalf("long token not matched: %v", got)
	}
	if _, err := tokenizer.DecodeTokensFromBinaryChecked([]byte{1}); err == nil {
		t.Fatal("accepted odd token length")
	}
	if _, err := tokenizer.DecodeChecked([]Token{65535}); err == nil {
		t.Fatal("accepted unknown token ID")
	}
}
func TestConcurrentTrainingAndEncoding(t *testing.T) {
	tokenizer := NewTokenizer()
	var workers sync.WaitGroup
	for i := 0; i < 4; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := 0; j < 30; j++ {
				text := "the ship has more power"
				if tokenizer.Decode(tokenizer.Encode(text)) != text {
					t.Error("roundtrip mismatch")
				}
				tokenizer.Train([]string{text, text}, 1)
				_ = tokenizer.ExportVocab()
			}
		}()
	}
	workers.Wait()
}
func FuzzRoundTrip(f *testing.F) {
	f.Add("the ship")
	f.Add("🚀")
	f.Fuzz(func(t *testing.T, text string) {
		tokenizer := NewTokenizer()
		if tokenizer.Decode(tokenizer.Encode(text)) != text {
			t.Fatal("roundtrip mismatch")
		}
	})
}
