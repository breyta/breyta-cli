package cli

import (
	"fmt"
	"regexp"
)

var (
	flowDiscoverPublicTrueRe     = regexp.MustCompile(`(?s):discover\s*\{[^}]*:public\s+true`)
	flowMarketplaceVisibleTrueRe = regexp.MustCompile(`(?s):marketplace\s*\{[^}]*:visible\s+true`)
)

func flowSourceRequestsPublicAccess(source string) bool {
	return flowDiscoverPublicTrueRe.MatchString(source) || flowMarketplaceVisibleTrueRe.MatchString(source)
}

func publicAccessConfirmationError(action string) error {
	return fmt.Errorf("%s can make this flow accessible to all Breyta users; ask the flow author for explicit approval, verify the flow is installable-ready, then rerun with --allow-public-access", action)
}
