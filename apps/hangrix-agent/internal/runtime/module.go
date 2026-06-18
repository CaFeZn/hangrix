package runtime

import (
	"time"

	"github.com/hangrix/hangrix/apps/hangrix-agent/internal/config"
	"github.com/hangrix/hangrix/apps/hangrix-agent/internal/llm"
	"github.com/hangrix/hangrix/apps/hangrix-agent/internal/prompt"
	"github.com/hangrix/hangrix/apps/hangrix-agent/internal/tools"
	"github.com/hangrix/hangrix/apps/hangrix-agent/internal/tools/local"
	"github.com/hangrix/hangrix/pkg/ioc"
)

// Deps pulls in every direct dependency the runtime loop needs. This
// is the deepest node in the agent's dependency graph below *app.App:
// any module that wires something the loop reaches transitively must
// be loaded into the container before this one's NewProvider runs.
//
// The transport (in / out) is constructed inline by NewProvider rather
// than wired through ioc — it's a single Connect client keyed on Config
// fields and has no fan-out, so a Deps entry would just add ceremony.
// Tests construct Loop directly via NewLoop with *ipc.Reader/*ipc.Writer
// over io.Pipe so the Connect wiring is exercised only in production.
type Deps struct {
	Cfg       *config.Config
	LLM       *llm.Client
	Registry  *tools.Registry
	Assembled *prompt.Assembled
	// Async is the lifecycle handle for local async work (background bash
	// tasks, sleep timers, etc.). The runtime drains its NotificationCh
	// into the LLM context at every drain point (round boundary, idle
	// wait) and calls Cleanup on shutdown so unfinished work doesn't
	// outlive the agent process.
	Async local.AsyncLifecycle
}

func NewProvider(deps *Deps) *Loop {
	transport, err := newConnectTransport(deps.Cfg.PlatformBaseURL, deps.Cfg.SessionID, deps.Cfg.SessionToken)
	if err != nil {
		// Same fail-fast policy as config.NewConfig — a misconfigured
		// transport means the agent has nothing to do; one stderr line +
		// process exit is cleaner than retrying every RPC and printing
		// the same parse error over and over.
		panic(err)
	}
	loop := NewLoop(
		transport,
		transport,
		deps.LLM,
		deps.Cfg.Model,
		deps.Registry,
		deps.Assembled.Prompt,
		deps.Async,
		deps.Cfg.CompactTokenThreshold,
		time.Duration(deps.Cfg.LLMReasoningTimeoutSeconds)*time.Second,
		deps.Cfg.LLMReasoningTimeoutRetries,
		deps.Cfg.LLMReasoningEffort,
		deps.Cfg.LLMThinking,
	)

	// Hold the StreamInputs idle grace open while either:
	//   1. local async work is pending (sleep timers, background bash), or
	//   2. the current LLM/tool turn is still running.
	//
	// (2) matters because the stream's idle timer is anchored to the last
	// inbound server frame, not to local activity. A long turn that ends by
	// scheduling sleep would otherwise inherit a stale EOF computed mid-turn
	// and die immediately after the tool call.
	transport.SetKeepAlive(loop.holdWakeOpen)
	return loop
}

func Module() *ioc.Module {
	m := ioc.NewModule()
	m.Provide(NewProvider).ToSelf()
	return m
}
