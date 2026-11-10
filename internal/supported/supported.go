// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

// Package supported answers version questions through the
// supported-versions-action module. It is the only package in this
// repository that imports the module's pkg/versions, so the extractors
// share one way of building a resolver and one set of settings.
//
// The module keeps the supported range as data in its catalog, reads
// the upstream release facts once per job through a runner cache, and
// falls back to a snapshot embedded in the binary when the network is
// unavailable, so no release of this action is needed when a new
// release line ships.
package supported

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"time"

	"github.com/lfreleng-actions/supported-versions-action/pkg/cache"
	"github.com/lfreleng-actions/supported-versions-action/pkg/catalog"
	"github.com/lfreleng-actions/supported-versions-action/pkg/constraint"
	"github.com/lfreleng-actions/supported-versions-action/pkg/release"
	"github.com/lfreleng-actions/supported-versions-action/pkg/snapshot"
	"github.com/lfreleng-actions/supported-versions-action/pkg/source"
	"github.com/lfreleng-actions/supported-versions-action/pkg/versions"
)

// Report is the module's matrix report, re-exported so callers need
// not import pkg/versions themselves.
type Report = versions.Report

// OutcomeOutOfRangeFallback is the constraint outcome the module
// reports when a constraint allows no line of the matrix and the
// matrix is returned unconstrained.
const OutcomeOutOfRangeFallback = constraint.OutcomeOutOfRangeFallback

// ErrConstraintSyntax reports constraint text that does not parse.
// The error Matrix returns for such text wraps it.
var ErrConstraintSyntax = constraint.ErrSyntax

// StatusEOL is the status of a report line past its end-of-life.
const StatusEOL = release.StatusEOL

// MatrixResolver answers matrix questions for a tool. A *Resolver
// satisfies it; tests inject a recorded report.
type MatrixResolver interface {
	Matrix(ctx context.Context, tool, constraint string) (Report, error)
}

// SourceActionsManifest is the catalog source type of an actions
// versions manifest, such as actions/go-versions.
const SourceActionsManifest = snapshot.TypeActionsManifest

// SourceEndOfLife is the catalog source type of endoflife.date
// lifecycle data.
const SourceEndOfLife = snapshot.TypeEndOfLife

// SourceEntry says where one catalog source's facts in a report came
// from.
type SourceEntry = versions.SourceEntry

// Kind says where a report's release facts came from.
type Kind = versions.Kind

// Report source kinds, from the freshest to the worst: facts fetched
// just now, read from a cache entry younger than its maximum age, read
// from an older cache entry because the upstream failed, read from the
// snapshot embedded in the module, and a source nothing could supply.
const (
	KindLive        = versions.KindLive
	KindCache       = versions.KindCache
	KindStale       = versions.KindStale
	KindSnapshot    = versions.KindSnapshot
	KindUnavailable = versions.KindUnavailable
)

// CatalogWithoutSource returns the module's embedded catalog with
// every source of sourceType removed from tool, a catalog id. A
// resolver given it never reads that source for tool, so the source's
// failures cannot fail tool's report.
func CatalogWithoutSource(tool, sourceType string) (*catalog.Catalog, error) {
	cat, err := catalog.Embedded()
	if err != nil {
		return nil, fmt.Errorf("supported: %w", err)
	}
	t, ok := cat.Tools[tool]
	if !ok {
		return nil, fmt.Errorf("supported: unknown catalog tool %q", tool)
	}
	t.Sources = slices.DeleteFunc(slices.Clone(t.Sources), func(s catalog.Source) bool {
		return s.Type == sourceType
	})
	cat.Tools[tool] = t
	return cat, nil
}

// NewReleased returns a Resolver configured by s that reads tool's
// release facts without its actions versions manifest and admits a
// line once released, as the lifecycle data says. s.Catalog is
// replaced by the embedded catalog without tool's manifest source, and
// s.Override, whose other fields are kept, sets the admission. The
// module fails a whole report when a manifest answers 404 or does not
// parse, so a resolver built here keeps valid lifecycle data when the
// manifest breaks.
func NewReleased(tool string, s Settings) (*Resolver, error) {
	cat, err := CatalogWithoutSource(tool, SourceActionsManifest)
	if err != nil {
		return nil, err
	}
	s.Catalog = cat
	override := catalog.PolicyOverride{}
	if s.Override != nil {
		override = *s.Override
	}
	admit := catalog.AdmitReleased
	override.Admit = &admit
	s.Override = &override
	return New(s)
}

// Settings configure a Resolver. They come from the action inputs
// python_offline_mode, python_eol_timeout and python_eol_max_retries,
// which govern every language resolved through this package: Python,
// Go and Rust.
type Settings struct {
	// Offline never touches the network: release facts come from the
	// runner cache or the snapshot embedded in the module.
	Offline bool
	// NoCache never opens the runner cache. With Offline, release
	// facts come from the embedded snapshot alone, which no cached
	// response, however malformed, can override.
	NoCache bool
	// Timeout bounds each upstream request; zero uses the module
	// default.
	Timeout time.Duration
	// Retries is the number of extra attempts after the first; zero
	// or less disables retries.
	Retries int
	// Catalog is the tool catalog; nil uses the module's embedded
	// catalog.
	Catalog *catalog.Catalog
	// Override replaces policy fields of every catalog tool; nil
	// applies none.
	Override *catalog.PolicyOverride
	// Now returns the current time; nil uses time.Now.
	Now func() time.Time
	// EndOfLifeBaseURL overrides the endoflife.date API root. It
	// exists for tests.
	EndOfLifeBaseURL string
	// ManifestBaseURL overrides the root of the actions versions
	// manifests. It exists for tests.
	ManifestBaseURL string
	// RustBaseURL overrides the Rust distribution root the stable
	// channel manifest is read from. It exists for tests.
	RustBaseURL string
}

// Resolver resolves version matrices through the module.
type Resolver struct {
	resolver *versions.Resolver
	cache    *cache.Cache
	offline  bool
}

// warnings receives the warnings New and Matrix emit. Tests replace it.
var warnings io.Writer = os.Stderr

func warnf(format string, args ...any) {
	_, _ = fmt.Fprintf(warnings, "[WARNING] "+format+"\n", args...)
}

// New returns a Resolver configured by s. It opens the runner cache
// with cache.Open, which resolves $SUPPORTED_VERSIONS_CACHE_DIR, then
// $RUNNER_TEMP on a GitHub Actions runner, then the user cache, unless
// s.NoCache is set. A cache that will not open is a warning and the
// resolver runs without one.
func New(s Settings) (*Resolver, error) {
	retries := s.Retries
	if retries <= 0 {
		// The module reads zero as "use the default" and a negative
		// value as "no retries"; this action's input reads zero as
		// "no retries".
		retries = -1
	}

	var c *cache.Cache
	if !s.NoCache {
		var err error
		c, err = cache.Open()
		if err != nil {
			warnf("Cannot open the supported-versions cache (%v); resolving without one", err)
			c = nil
		}
	}

	r, err := versions.New(versions.Options{
		Catalog:  s.Catalog,
		Override: s.Override,
		Fetch: source.Options{
			Cache:   c,
			Timeout: s.Timeout,
			Retries: retries,
			Offline: s.Offline,
		},
		Now:              s.Now,
		EndOfLifeBaseURL: s.EndOfLifeBaseURL,
		ManifestBaseURL:  s.ManifestBaseURL,
		RustBaseURL:      s.RustBaseURL,
	})
	if err != nil {
		if c != nil {
			_ = c.Close()
		}
		return nil, fmt.Errorf("supported: %w", err)
	}
	return &Resolver{resolver: r, cache: c, offline: s.Offline}, nil
}

// Matrix resolves the matrix of tool, a catalog id or alias, narrowed
// by constraint in the tool's constraint syntax; "" applies none.
// Online, release facts served from a stale cache entry or the
// embedded snapshot raise a warning, since they mean the upstream
// could not be reached.
func (r *Resolver) Matrix(ctx context.Context, tool, constraint string) (Report, error) {
	rep, err := r.resolver.Matrix(ctx, tool, versions.MatrixRequest{Constraint: constraint})
	if err != nil {
		return rep, err
	}
	if !r.offline && (rep.Source.Kind == versions.KindStale || rep.Source.Kind == versions.KindSnapshot) {
		warnf("Could not fetch live %s release data; using %s data", rep.Name, rep.Source.Kind)
	}
	return rep, nil
}

// Close releases the cache the resolver holds open. The resolver is
// unusable afterwards.
func (r *Resolver) Close() error {
	if r.cache == nil {
		return nil
	}
	return r.cache.Close()
}
