package controller

type PlanLimits struct {
	ScrapedPages int
	Documents    int
}

func limitsForPlan(plan string) PlanLimits {
	switch plan {
	case "basic":
		return PlanLimits{ScrapedPages: 50, Documents: 10}
	case "enterprise":
		return PlanLimits{ScrapedPages: 400, Documents: 50}
	default:
		return PlanLimits{ScrapedPages: 30, Documents: 5}
	}
}
