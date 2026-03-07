package controller

// PlanLimits holds all feature limits and flags for a subscription plan.
// Zero values for numeric limits mean "unlimited".
type PlanLimits struct {
	ScrapedPages  int      // max URL scrape pages; 0 = unlimited
	Documents     int      // max documents; 0 = unlimited
	DocumentsMB   int      // max document upload size MB; 0 = unlimited
	Conversations int      // max messages/month; 0 = unlimited
	ChatHistory   int      // max chat sessions shown; 0 = unlimited
	Leads         int      // max leads stored; 0 = unlimited
	HideBranding  bool     // can hide "Powered by KonvoqAI" from widget
	HasCRM        bool     // CRM pipeline access
	HasFollowUp   bool     // auto follow-up emails
	HasHybrid     bool     // hybrid AI+human handoff inbox
	HasFlows      bool     // conversation flow builder
	HasPersona    bool     // AI persona / role builder
	HasNavigation bool     // widget navigation builder
	Roles         []string // available AI roles for persona
}

func limitsForPlan(plan string) PlanLimits {
	switch plan {
	case "basic":
		return PlanLimits{
			ScrapedPages:  45,
			Documents:     25,
			DocumentsMB:   10,
			Conversations: 1500,
			ChatHistory:   50,
			Leads:         15,
			Roles:         []string{"professional", "casual"},
		}
	case "pro":
		return PlanLimits{
			ScrapedPages:  150,
			Documents:     80,
			DocumentsMB:   10,
			Conversations: 5000,
			HideBranding:  true,
			HasCRM:        true,
			HasFollowUp:   true,
			HasHybrid:     true,
			HasFlows:      true,
			HasPersona:    true,
			HasNavigation: true,
			Roles:         []string{"professional", "casual", "sales", "marketing", "hr"},
		}
	case "enterprise":
		return PlanLimits{
			ScrapedPages:  500,
			HideBranding:  true,
			HasCRM:        true,
			HasFollowUp:   true,
			HasHybrid:     true,
			HasFlows:      true,
			HasPersona:    true,
			HasNavigation: true,
			Roles:         []string{"professional", "casual", "sales", "marketing", "hr", "custom"},
		}
	default: // free
		return PlanLimits{
			ScrapedPages:  15,
			Documents:     10,
			DocumentsMB:   5,
			Conversations: 300,
			ChatHistory:   5,
			Leads:         3,
			Roles:         []string{},
		}
	}
}
