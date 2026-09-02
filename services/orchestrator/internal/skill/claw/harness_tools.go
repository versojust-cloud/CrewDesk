package claw

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dreamwaver/dreamwaver/services/orchestrator/internal/harness"
	"github.com/dreamwaver/dreamwaver/services/orchestrator/internal/tool"
)

const clawSessionDependency = "claw.session"

type clawToolFactory struct {
	name   string
	policy harness.ToolPolicy
	wired  func() bool
	build  func(*Session) tool.Tool
}

func (f *clawToolFactory) Name() string { return f.name }

func (f *clawToolFactory) Available(ctx context.Context, scope harness.ToolScope) harness.Availability {
	if err := ctx.Err(); err != nil {
		return harness.Availability{Available: false, Reason: err.Error()}
	}
	if f.wired != nil && !f.wired() {
		return harness.Availability{Available: false, Reason: "capability backend not configured"}
	}
	return harness.Availability{Available: true}
}

func (f *clawToolFactory) Build(ctx context.Context, scope harness.ToolScope) (harness.ToolInstance, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	session, ok := harness.Dependency[*Session](scope, clawSessionDependency)
	if !ok || session == nil {
		return nil, errors.New("claw session dependency is required")
	}
	instance := f.build(session)
	if instance == nil {
		return nil, fmt.Errorf("tool %q factory returned nil", f.name)
	}
	return instance, nil
}

func (f *clawToolFactory) Policy() harness.ToolPolicy { return f.policy }

// RegisterHarnessTools installs the Claw skill pack's executable capabilities
// into the shared Harness catalog. Factories retain the Runner dependencies,
// while each Build receives the active run's Session through ToolScope.
func RegisterHarnessTools(catalog *harness.ToolCatalog, runner *Runner) error {
	if catalog == nil || runner == nil {
		return errors.New("claw harness tool catalog and runner are required")
	}

	artifactPolicy := harness.ToolPolicy{SideEffect: true}
	factories := []*clawToolFactory{
		{
			name:   "web_search",
			policy: harness.ToolPolicy{RequiresNetwork: true},
			wired:  func() bool { return strings.TrimSpace(runner.TavilyKey) != "" },
			build:  func(*Session) tool.Tool { return tool.NewTavilySearch(runner.TavilyKey) },
		},
		{
			name:   "find_kol",
			policy: harness.ToolPolicy{RequiresNetwork: true},
			wired:  func() bool { return runner.KOL != nil },
			build: func(session *Session) tool.Tool {
				return &FindKOL{Finder: runner.KOL, Session: session, Emitter: runner.Emitter}
			},
		},
		{
			name:   "code_execute",
			policy: harness.ToolPolicy{SandboxProfile: "isolated-code"},
			wired:  func() bool { return runner.SandboxClient != nil },
			build:  func(*Session) tool.Tool { return tool.CodeExecute{Client: runner.SandboxClient} },
		},
		{
			name:   "generate_image",
			policy: artifactPolicy,
			wired:  func() bool { return runner.ImagesEnabled && runner.Images != nil },
			build: func(session *Session) tool.Tool {
				return &GenerateImage{Images: runner.Images, Session: session, Emitter: runner.Emitter}
			},
		},
		{
			name:   "generate_video",
			policy: artifactPolicy,
			wired:  func() bool { return runner.Video != nil },
			build: func(session *Session) tool.Tool {
				return &GenerateVideo{Video: runner.Video, Session: session, Emitter: runner.Emitter}
			},
		},
		{
			name:   "edit_image",
			policy: artifactPolicy,
			wired:  func() bool { return runner.Editor != nil },
			build: func(session *Session) tool.Tool {
				return &EditImage{Editor: runner.Editor, Session: session, Emitter: runner.Emitter}
			},
		},
		{
			name:   "generate_poster",
			policy: artifactPolicy,
			wired:  func() bool { return runner.ImagesEnabled && runner.Images != nil },
			build: func(session *Session) tool.Tool {
				return &GeneratePoster{Images: runner.Images, Session: session, Emitter: runner.Emitter}
			},
		},
		{
			name:   "generate_storybook",
			policy: artifactPolicy,
			wired:  func() bool { return runner.ImagesEnabled && runner.Images != nil },
			build: func(session *Session) tool.Tool {
				return &GenerateStorybook{Images: runner.Images, Session: session, Emitter: runner.Emitter}
			},
		},
		{
			name:   "generate_variants",
			policy: artifactPolicy,
			wired:  func() bool { return runner.Variants != nil },
			build: func(session *Session) tool.Tool {
				return &GenerateVariantsTool{Variants: runner.Variants, Session: session, Emitter: runner.Emitter}
			},
		},
		{
			name:   "generate_game",
			policy: artifactPolicy,
			wired:  func() bool { return runner.Game != nil },
			build: func(session *Session) tool.Tool {
				return &GenerateGame{Game: runner.Game, Session: session, Emitter: runner.Emitter}
			},
		},
		{
			name:   "edit_deck",
			policy: artifactPolicy,
			wired:  func() bool { return runner.DeckEditor != nil },
			build: func(session *Session) tool.Tool {
				return &EditDeck{Editor: runner.DeckEditor, Session: session, Emitter: runner.Emitter}
			},
		},
		{
			name:   "write_document",
			policy: artifactPolicy,
			build: func(session *Session) tool.Tool {
				return &WriteDocument{Session: session, Emitter: runner.Emitter}
			},
		},
		{
			name:   "generate_deck",
			policy: artifactPolicy,
			wired:  func() bool { return runner.Pipeline != nil },
			build: func(session *Session) tool.Tool {
				return &GenerateDeck{Pipeline: runner.Pipeline, Session: session, Emitter: runner.Emitter}
			},
		},
	}

	for _, factory := range factories {
		if err := catalog.Register(factory); err != nil {
			return err
		}
	}
	return nil
}
