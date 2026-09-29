// Package triage sorts email texts among the configured categories, with
// indecis in open mode: each category is described by its name, its
// description and its example emails, and compared with the email.
package triage

import (
	"context"

	"github.com/bornholm/indecis"

	"github.com/bornholm/indecis-email-triage/internal/config"
)

// Result is the sort of one email.
type Result struct {
	Category   config.Category
	Confidence float64            // probability of the category
	Margin     float64            // gap with the mean of the others
	Probs      map[string]float64 // per category name
}

// Classifier holds the model and the prepared categories.
type Classifier struct {
	set  *indecis.CandidateSet
	m    *indecis.Model
	cats map[string]config.Category
}

// New loads the model of the configuration and prepares its categories.
func New(ctx context.Context, cfg *config.Config) (*Classifier, error) {
	m, err := indecis.Open(cfg.Model, indecis.WithInt8())
	if err != nil {
		return nil, err
	}
	c := &Classifier{m: m, cats: map[string]config.Category{}}
	cands := make([]indecis.Candidate, len(cfg.Categories))
	for i, cat := range cfg.Categories {
		cands[i] = indecis.Candidate{Name: cat.Name, Description: cat.Description, Examples: cat.Examples}
		c.cats[cat.Name] = cat
	}
	if c.set, err = m.PrepareCandidates(ctx, cands); err != nil {
		return nil, err
	}
	return c, nil
}

// Classify sorts texts (see mailtext.Email.Text).
func (c *Classifier) Classify(ctx context.Context, texts ...string) ([]Result, error) {
	answers, err := c.m.ChooseIn(ctx, c.set, texts...)
	if err != nil {
		return nil, err
	}
	out := make([]Result, len(answers))
	for i, a := range answers {
		out[i] = Result{Category: c.cats[a.Choice], Confidence: a.Confidence, Margin: a.Margin, Probs: a.Probs}
	}
	return out, nil
}
