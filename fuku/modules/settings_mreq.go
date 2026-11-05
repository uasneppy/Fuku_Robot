package modules

import "github.com/uasneppy/fuku_robot/fuku/db/models"

const (
	// settingsPageMreq is the p value of the Member requirements page.
	settingsPageMreq = "m"
	// settingsMreqControlJoin is the t value of the Check on join switch.
	settingsMreqControlJoin = "j"
)

// renderSettingsMreqPage is a compile-only stand-in until the page is built.
func renderSettingsMreqPage(_ settingsView, _ *models.ChatRequirements, _ string) settingsScreen {
	return settingsScreen{}
}
