package qualityrender

import (
	"encoding/json"
	"os"
	"testing"
)

const modelAHTML = `<html><button aria-pressed="false">Play</button><svg aria-labelledby="title description"><title id="title">Pelican</title><desc id="description">Bike</desc><defs><path id="far-leg"/><path id="near-leg"/><path id="far-foot"/><path id="near-foot"/><path id="wheel-turn"/><path id="crank-turn"/></defs><use href="#far-leg"/><use href="#near-leg"/><use href="#far-foot"/><use href="#near-foot"/><use href="#wheel-turn"/><use href="#crank-turn"/></svg><script>const media=matchMedia('prefers-reduced-motion'); document.addEventListener('visibilitychange',()=>{}); function tick(){Math.cos(1); requestAnimationFrame(tick)}</script></html>`

type referenceFixture struct {
	Name     string `json:"name"`
	HTML     string `json:"html"`
	Expected struct {
		Score          float64  `json:"score"`
		RawPoints      float64  `json:"raw_points"`
		IsModelA       bool     `json:"is_model_a"`
		MatchedSignals []string `json:"matched_signals"`
		MissingSignals []string `json:"missing_signals"`
		Threshold      float64  `json:"threshold"`
	} `json:"expected"`
}

func TestMatchHTMLReferenceSignalsAndThreshold(t *testing.T) {
	got, err := MatchHTML(modelAHTML, 55)
	if err != nil {
		t.Fatal(err)
	}
	if got.RawPoints != 100 || got.Score != 100 || !got.IsModelA {
		t.Fatalf("match=%+v", got)
	}
	got, err = MatchHTML(`<svg class="sun"></svg>`, 55)
	if err != nil {
		t.Fatal(err)
	}
	if got.RawPoints != 7 || got.IsModelA {
		t.Fatalf("negative match=%+v", got)
	}
	got, err = MatchHTML(`<svg></svg>`, 0)
	if err != nil || !got.IsModelA {
		t.Fatalf("boundary=%+v err=%v", got, err)
	}
}

func TestMatchHTMLRejectsNonSVGAndBadThreshold(t *testing.T) {
	if _, err := MatchHTML(`<html><script>"<svg>"</script></html>`, 55); err == nil {
		t.Fatal("script string accepted")
	}
	if _, err := MatchHTML(`<svg/>`, 101); err == nil {
		t.Fatal("bad threshold accepted")
	}
}

func TestMatchHTMLTitleDescriptionUsesStructuralIDs(t *testing.T) {
	got, err := MatchHTML(`<svg aria-labelledby="svg-title svg-desc"><title id="svg-title">Pelican</title><desc id="svg-desc">Bike</desc></svg>`, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, signal := range got.MatchedSignals {
		if signal == "title_desc_aria_labelledby" {
			return
		}
	}
	t.Fatalf("structural title/desc signal missing: %+v", got)
}

func TestMatchHTMLMatchesReferenceFixtures(t *testing.T) {
	raw, err := os.ReadFile("testdata/fingerprint_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []referenceFixture
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		got, err := MatchHTML(tc.HTML, tc.Expected.Threshold)
		if err != nil {
			t.Fatalf("%s: %v", tc.Name, err)
		}
		if got.Score != tc.Expected.Score || float64(got.RawPoints) != tc.Expected.RawPoints || got.IsModelA != tc.Expected.IsModelA {
			t.Fatalf("%s: got=%+v expected=%+v", tc.Name, got, tc.Expected)
		}
		if len(got.MatchedSignals) != len(tc.Expected.MatchedSignals) || len(got.MissingSignals) != len(tc.Expected.MissingSignals) {
			t.Fatalf("%s: signals differ got=%+v expected=%+v", tc.Name, got, tc.Expected)
		}
	}
}
