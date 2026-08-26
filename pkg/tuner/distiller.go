package tuner

// DistillRule synthesizes a clean expr rule from optimal threshold and friction keywords,
// and validates that it compiles cleanly with expr.Compile.
func DistillRule(optimalThreshold int, highFrictionKws []string) (string, error) {
	return RewriteRuleAST("", optimalThreshold, highFrictionKws, false, false)
}

// DistillRuleWithContext synthesizes an optimal rule considering existing tier guardrails and empirical modality constraints.
func DistillRuleWithContext(existingWhen string, optimalThreshold int, highFrictionKws []string, restrictImages, restrictTools bool) (string, error) {
	return RewriteRuleAST(existingWhen, optimalThreshold, highFrictionKws, restrictImages, restrictTools)
}
