package strategy

import (
	"fmt"

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
}

// NewExprEvaluator compiles all tier expressions in advance for nanosecond execution.
func NewExprEvaluator(tiers []contract.Tier, defaultTier contract.Tier) (*ExprEvaluator, error) {
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

	return &ExprEvaluator{
		compiled:    compiledList,
		defaultTier: defaultTier,
	}, nil
}

// SelectTier runs the bytecode program against reqCtx and returns the first matching tier.
func (e *ExprEvaluator) SelectTier(reqCtx contract.RequestContext) (contract.Tier, error) {
	for _, ct := range e.compiled {
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
