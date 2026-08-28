package strategy

import (
	"fmt"
	"strings"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

type compiledTier struct {
	tier    contract.Tier
	program *vm.Program
}

// ExprEvaluator evaluates 1..N tiers sequentially using compiled expr bytecode.
type ExprEvaluator struct {
	compiled    []compiledTier
	defaultTier contract.Tier
	providers   map[string]contract.ProviderConfig
}

// NewExprEvaluator compiles all tier expressions in advance for nanosecond execution.
func NewExprEvaluator(tiers []contract.Tier, defaultTier contract.Tier, providers ...map[string]contract.ProviderConfig) (*ExprEvaluator, error) {
	compiledList := make([]compiledTier, 0, len(tiers))

	for _, t := range tiers {
		if t.When == "" {
			continue
		}
		// Compile expression against the RequestContext struct schema
		program, err := expr.Compile(t.When, expr.Env(contract.RequestContext{}))
		if err != nil {
			return nil, fmt.Errorf("failed to compile expr for tier '%s' (%s): %w", t.Name, t.When, err)
		}
		compiledList = append(compiledList, compiledTier{
			tier:    t,
			program: program,
		})
	}

	var provMap map[string]contract.ProviderConfig
	if len(providers) > 0 {
		provMap = providers[0]
	}

	return &ExprEvaluator{
		compiled:    compiledList,
		defaultTier: defaultTier,
		providers:   provMap,
	}, nil
}

// SelectTier runs the bytecode program against reqCtx and returns the first matching tier.
func (e *ExprEvaluator) SelectTier(reqCtx contract.RequestContext) (contract.Tier, error) {
	// 1. Forced Model Directive
	if reqCtx.ForcedModel != "" {
		return contract.Tier{
			Name:     "Forced Model",
			Model:    reqCtx.ForcedModel,
			Provider: e.defaultTier.Provider,
		}, nil
	}

	// 2. Forced Tier Directive
	if reqCtx.ForcedTier != "" {
		forced := strings.ToLower(reqCtx.ForcedTier)
		switch forced {
		case "local":
			for _, ct := range e.compiled {
				if provCfg, ok := e.providers[ct.tier.Provider]; ok && provCfg.IsLocal() {
					return ct.tier, nil
				}
			}
			if provCfg, ok := e.providers[e.defaultTier.Provider]; ok && provCfg.IsLocal() {
				return e.defaultTier, nil
			}
			if len(e.compiled) > 0 {
				return e.compiled[0].tier, nil
			}
			return e.defaultTier, nil

		case "cloud", "frontier":
			return e.defaultTier, nil

		case "reasoning":
			for _, ct := range e.compiled {
				name := strings.ToLower(ct.tier.Name)
				model := strings.ToLower(ct.tier.Model)
				if strings.Contains(name, "reasoning") || strings.Contains(model, "r1") || strings.Contains(model, "qwq") || strings.Contains(model, "o1") || strings.Contains(model, "o3") {
					return ct.tier, nil
				}
			}
			return contract.Tier{}, fmt.Errorf("no reasoning tier configured in config.yaml")

		default:
			// Exact or case-insensitive tier name match
			for _, ct := range e.compiled {
				if strings.EqualFold(ct.tier.Name, reqCtx.ForcedTier) {
					return ct.tier, nil
				}
			}
			if strings.EqualFold(e.defaultTier.Name, reqCtx.ForcedTier) {
				return e.defaultTier, nil
			}
			return contract.Tier{}, fmt.Errorf("forced tier %q not found in config.yaml", reqCtx.ForcedTier)
		}
	}

	// 3. Normal AST Bytecode Evaluation
	for _, ct := range e.compiled {
		// Guard: If model has a physical context window limit and prompt exceeds it, skip tier
		if ct.tier.MaxContext > 0 && reqCtx.Tokens > ct.tier.MaxContext {
			continue
		}

		output, err := expr.Run(ct.program, reqCtx)
		if err != nil {
			// Log error or continue to next tier
			continue
		}
		matched, ok := output.(bool)
		if ok && matched {
			return ct.tier, nil
		}
	}
	return e.defaultTier, nil
}
