package typesafe

import (
	"encoding/json"
	"strings"
	"testing"
)

func benchmarkLegacyValidate(body []byte) (string, error) {
	var e systemOneEnvelope
	if err := json.Unmarshal(body, &e); err != nil {
		return "", err
	}
	if err := checkSystemOneObjectKeys(body, "request", systemOneRequestFields); err != nil {
		return "", err
	}
	model, err := requiredString(e.Model, "model")
	if err != nil {
		return "", err
	}
	if err = validateStringObjectOrArray(e.State, "state"); err != nil {
		return "", err
	}
	var q map[string]json.RawMessage
	if err = json.Unmarshal(e.Questions, &q); err != nil {
		return "", err
	}
	if err = checkSystemOneObjectKeys(e.Questions, "questions", nil); err != nil {
		return "", err
	}
	for id, v := range q {
		if err = validateQuestion(id, v); err != nil {
			return "", err
		}
	}
	return model, nil
}
func BenchmarkSystemOneValidation(b *testing.B) {
	for _, size := range []int{1024, 256 << 10} {
		body, _ := json.Marshal(map[string]any{"model": "jev-latest", "state": strings.Repeat("x", size), "questions": map[string]any{"q": map[string]any{"type": "noul", "instructions": "Check"}}})
		for _, impl := range []struct {
			name string
			f    func([]byte) (string, error)
		}{{"upstream", benchmarkLegacyValidate}, {"adapted", ValidateSystemOneRequest}} {
			b.Run(impl.name+"/"+map[int]string{1024: "1KiB", 256 << 10: "256KiB"}[size], func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					if _, err := impl.f(body); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
