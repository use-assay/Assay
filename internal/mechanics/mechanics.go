// Package mechanics classifies trap risk for a Stellar asset from the
// ledger's own rules.
//
// The design rule here is that checks perform no I/O. Everything a check needs
// is fetched once, up front, into a Subject; a check is then a pure function
// from Subject to Finding. That keeps checks deterministic, makes them testable
// from fixtures with no network, and confines every fetcher to its own package
// where it can be extracted later.
package mechanics

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/use-assay/assay/internal/horizon"
	"github.com/use-assay/assay/internal/sep1"
	"github.com/use-assay/assay/internal/stellarexpert"
)

// Asset identifies a classic Stellar asset.
type Asset struct {
	Code   string `json:"code"`
	Issuer string `json:"issuer"`
}

// String renders the canonical CODE-ISSUER form.
func (a Asset) String() string { return a.Code + "-" + a.Issuer }

// Evidence is a single attributed claim from a named source.
//
// Every consumed signal enters the report through this type. Attribution is
// structural rather than conventional: because a check can only surface an
// outside claim by constructing an Evidence with its source and URL, there is
// no code path that renders someone else's data as an Assay conclusion.
type Evidence struct {
	Source      string    `json:"source"`
	URL         string    `json:"url"`
	Claim       string    `json:"claim"`
	RetrievedAt time.Time `json:"retrieved_at"`
}

// Finding is one check's result.
type Finding struct {
	Check    string   `json:"check"`
	Title    string   `json:"title"`
	Severity Severity `json:"severity"`
	// Escalation marks a finding whose severity comes from reputation rather
	// than from issuer capability. The engine keeps these separate so that the
	// capability-only base severity stays auditable.
	Escalation bool `json:"escalation"`
	// Reasoning always states the raw capability in plain language, whatever
	// the severity works out to.
	Reasoning string `json:"reasoning"`
	// Mechanics are the bits this check observed.
	Mechanics Mechanic `json:"-"`
	// MechanicNames is the human-readable form of Mechanics.
	MechanicNames []string `json:"mechanics"`
	// Accountability is set only by the check that establishes it.
	Accountability *Accountability `json:"accountability,omitempty"`
	Evidence       []Evidence      `json:"evidence"`
}

// Subject is the pre-fetched state a check reasons over. Checks must not reach
// outside it.
type Subject struct {
	Asset  Asset
	Stat   *horizon.AssetStat
	Issuer *horizon.Account

	// Toml is the issuer's stellar.toml if it resolved. TomlErr records why it
	// did not, and is reported verbatim rather than being smoothed over.
	Toml    *sep1.Doc
	TomlURL string
	TomlErr string

	Directory    *stellarexpert.DirectoryEntry
	DirectoryURL string
	Blocked      *stellarexpert.BlockedDomain
	BlockedURL   string

	FetchedAt time.Time
}

// HomeDomain returns the issuer's advertised home_domain, if any.
func (s *Subject) HomeDomain() string {
	if s.Issuer == nil {
		return ""
	}
	return s.Issuer.HomeDomain
}

// Check is a single mechanic classifier. Implementations are pure functions
// over the Subject and must not perform I/O.
type Check interface {
	// ID is the stable identifier used in reports and issue tracking.
	ID() string
	// Describe states what the check concludes, and what it does not.
	Describe() string
	// Run classifies the subject.
	Run(ctx context.Context, s *Subject) (Finding, error)
}

// Report is the aggregated result of running every check over one asset.
type Report struct {
	Asset Asset `json:"asset"`

	// Severity is the final level: the capability base, raised by any
	// reputation escalation.
	Severity Severity `json:"severity"`
	// Base is the capability-only severity before escalation. Base and
	// Severity differ only when a curated source flagged this issuer, which
	// makes every escalation visible and auditable.
	Base Severity `json:"base_severity"`
	// Escalated reports whether reputation raised the level.
	Escalated bool `json:"escalated"`

	// Accountability is reported alongside severity, never folded into it.
	Accountability Accountability `json:"accountability"`

	Mechanics     Mechanic   `json:"-"`
	MechanicNames []string   `json:"mechanics"`
	Findings      []Finding  `json:"findings"`
	Evidence      []Evidence `json:"evidence"`
	ScannedAt     time.Time  `json:"scanned_at"`
}

// Engine runs a set of checks over a Subject.
type Engine struct {
	Checks []Check
}

// NewEngine returns an Engine with the default check set.
func NewEngine() *Engine {
	return &Engine{Checks: []Check{
		CapabilityCheck{},
		DomainCheck{},
		ReputationCheck{},
	}}
}

// Run executes every check and aggregates the results.
//
// Aggregation rules, which are the judgment model in code:
//
//   - Base severity is the maximum over non-escalation findings. That is pure
//     capability.
//   - Final severity is the maximum over all findings, so reputation can only
//     ever raise the level.
//   - Accountability is taken from whichever check establishes it and is not
//     permitted to influence either severity.
func (e *Engine) Run(ctx context.Context, s *Subject) (*Report, error) {
	rep := &Report{
		Asset:          s.Asset,
		Accountability: AccountabilityUnknown,
		ScannedAt:      s.FetchedAt,
		Findings:       []Finding{},
		Evidence:       []Evidence{},
	}

	for _, c := range e.Checks {
		f, err := c.Run(ctx, s)
		if err != nil {
			return nil, fmt.Errorf("check %s: %w", c.ID(), err)
		}
		f.MechanicNames = f.Mechanics.Names()

		if f.Escalation {
			if f.Severity > rep.Severity {
				rep.Severity = f.Severity
			}
		} else if f.Severity > rep.Base {
			rep.Base = f.Severity
		}

		if f.Accountability != nil {
			rep.Accountability = *f.Accountability
		}
		rep.Mechanics |= f.Mechanics
		rep.Findings = append(rep.Findings, f)
		rep.Evidence = append(rep.Evidence, f.Evidence...)
	}

	if rep.Base > rep.Severity {
		rep.Severity = rep.Base
	}
	rep.Escalated = rep.Severity > rep.Base
	rep.MechanicNames = rep.Mechanics.Names()

	sort.SliceStable(rep.Findings, func(i, j int) bool {
		return rep.Findings[i].Severity > rep.Findings[j].Severity
	})
	return rep, nil
}
