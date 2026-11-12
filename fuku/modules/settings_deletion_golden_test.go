//go:build testtools

package modules

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// settingsDelGoldenDigests are the SHA-256 digests of the text and the keyboard JSON of every Phase 16
// deletion screen, recorded once from the code as it was before Phase 17 touched the renderer
// (plan 17-09, M31). The optional row fields of Phase 17 are empty for every Phase 16 row, so none of
// these screens may change; the digests are never re-recorded to make a change pass.
var settingsDelGoldenDigests = map[string]string{
	"list b none can":      "07475a30811c1dde54438bef48a3cc000eb436e56cf38a97696c958d8229204a",
	"list b none cannot":   "f6c4ccfbe10fbb245de2180c484c4b804b31f43a4900f019b2f16b122f1475e6",
	"list b stored can":    "99fdc9644f6cf7dbc63538270d14335e2ef974103f00dc8d112867fbbe50f4c3",
	"list b stored cannot": "939943afaaec9fa03443f1367796e59147d392d57bf9572adb1fc9fa7b1bc85e",
	"list s none can":      "cbbde2512672edec4f7f296abfd65a2673c9057bca0b11d0319aeb087d186b17",
	"list s none cannot":   "5c9ef5f4a4fe8b0ca4f242b407b5ab666040aa06dd950ccae6aa76d4b86e0736",
	"list s stored can":    "65b416e708596a3e69b0953fb8acc588d6677f07829bf8d6542c144ff0290871",
	"list s stored cannot": "98b49926e23ddc131c3e61355746761003a15c2e8e69fd066a5beff71c3ca853",
	"row ba none can":      "3e368a19fe214e39188616d0b3ae536605db4291a5642758db28d9de05fa9acf",
	"row ba none cannot":   "ff0c9413fc8b47037bd10bbd9b6756cdfd4c6d959af01104f9706a8c166e24a4",
	"row ba stored can":    "90e49ea1ab54a8bbca04c8fefc52790da5beab5b44677f3c6a9fdd3041221b4d",
	"row ba stored cannot": "3a90eade7b8d0c4b2d9d2104de95b4cc5fd02baafedf4458ff01cedd961cb9e5",
	"row bg none can":      "b6db3c1dcb59546f97835dd7ba04a4fb8b4243a6a784ace6bafa974cdf03f207",
	"row bg none cannot":   "044d3ee0e687fc0595af4dd1fe2786cd0fe93b5c43ab3b704d64fab27dbd90cd",
	"row bg stored can":    "e82a2e05cc2a2f0cf9cba821e5d9d36eb1eb3d73b885b03ddce98db6030ee4f2",
	"row bg stored cannot": "6bc01eb897253e925601778c8bcaa119dbe202fd86af58954839b75af2044a3a",
	"row bm none can":      "49ea8db1a85274e29ba15b53dbcbe4f471dcb04c2e7fa4a89631f8de0f52f4a6",
	"row bm none cannot":   "0babfdcbd8f570b365f7342412bdd9c12809243ff2a576c95463c69f4086b5f5",
	"row bm stored can":    "9e7eaf547df10695e71aee15cadf0e275b958dc031cb6817fb19553949d6249c",
	"row bm stored cannot": "ba72022c168f08286267c44b22152a3b28dbb334e97595f2895c7f185b0ad2d8",
	"row br none can":      "5a3a45ef5afc7eed8c772d6aa3b22d4c04919cfe00919403c3293ee11d293f3f",
	"row br none cannot":   "54bd4a00b526c4d76e3a4f9cd90a6fe1ce52c513fc6dfae490f7f389f2b21460",
	"row br stored can":    "d2039d3a8b0022504869030cb7cb015c3bd54d5aa883d2f68b7da78a032a207c",
	"row br stored cannot": "b3aa6f16d60284d4534c2d4d60356e843e7458fee02a5aa567bf7795eb206c29",
	"row bw none can":      "750d704f013cadf7ddac7da4c2e5af8c8617e585898b0b5fe5f7aca18b3cdffc",
	"row bw none cannot":   "93ad544d9bafd239d34ff6b42ed5658613718fa9bb0376af583df627393b4ca3",
	"row bw stored can":    "0294d41b778682982a9bd31bff18d9e865808e02da3e07bed853697728423d9e",
	"row bw stored cannot": "c9c3eed4eb4be8e290e511f2e03a037e063583f2b9eb417618539fc0dae3f26a",
	"row sa none can":      "8bb9bfadec76c256002857e2c7ebba991598218ae9462d18b6902aa21c124278",
	"row sa none cannot":   "72e9dbe06cf56229b06c5768e448577beab2cf269db16893c34e8a0d9db75db1",
	"row sa stored can":    "6950e7f2f4828844adb6ac3f8e3465a8842653e86d3a46806b710b23d9020f91",
	"row sa stored cannot": "8f5bf1fc49cbc40b69ea5aaef0c0b118e654a52c5495e8dfa70819f0166d6c31",
	"row sb none can":      "1bb12ad998699ccec422f6ec2b04eecaa4c9611ef88ff4fa1768bc4f507cdf60",
	"row sb none cannot":   "44c328933830b8ca061ebf76eff24deef5e8731990b4e513408de69024bae081",
	"row sb stored can":    "0baca68b9fd7207c4aefd84b8d70d9f95869a6c26cf6628ac64235ce7a24fdf4",
	"row sb stored cannot": "6b6137bc2ca593ad8565616da0c0c915101ba1b1ad8aad2e15bb0023b32aefc9",
	"row sf none can":      "cbd533606c5cfd5ea037f9d8e5425f99bb7d16675fd2c7247a8fadca36082c4c",
	"row sf none cannot":   "8a9d7d99abe943d395b35814bbdb4bcc7b817734240b05a2bdd5c0b2eb5b9db6",
	"row sf stored can":    "016654d75c594c9f0802aec70260db5e7ee45457e316c5c88b9a450b792aadd2",
	"row sf stored cannot": "3899ecaa624fe2a4302f773e585e045260bcece3743eb2482229e3fc638971f7",
	"row sg none can":      "231191304d463a16fb6166c775d3e755feac992775a2be9fc2d58923eeb2a400",
	"row sg none cannot":   "0d25a749d502eb1d15d542d6f44e998e06797ad6f5c9b79347334e17e634ba03",
	"row sg stored can":    "68fdde742440fbf4cb89d35cffaae2d2325ba6a33722f28350892f1c693c4731",
	"row sg stored cannot": "06d04f2d6ff8cd518f1020242be3e48db67439d190644748e6e34b34c14cd8ca",
	"row sh none can":      "e8bcc5f3ca1bda44272ecc50f35a0c59635a122073be4b1f60b1a38070b936f1",
	"row sh none cannot":   "8c77b7e310ce028259eaec725c0f4f1a35ab59b94de6ded15c04219484f59f26",
	"row sh stored can":    "f051445d955c7df28de035be8e7880d9ef199bcb30f1da3ca48a0db8a649d39b",
	"row sh stored cannot": "76d00e8ce6fc8433625c963134cd0e0181367f13acd99addaabf623884cac324",
	"row sj none can":      "7aba03ed766a585bf14dbed353bab379c972cf9240f667a4f27e7a7b6b7ee5ac",
	"row sj none cannot":   "38bcddf142450cab0c9515b13b6a7ecfeb6ba0d314b6c4dfb96a2e2d02e98e4e",
	"row sj stored can":    "8418aefa629d282ee4383679e61ca9918b32f1a929a4bc8226794acc8c4f962c",
	"row sj stored cannot": "de02caf14e7449f674c4fd242ca3c26a386fc08c2e5529fa86121f172b836f43",
	"row sk none can":      "8c7e0414db38ab811c1be3f02ca66bf0babd580064f11e2d43b51c6a2834fd42",
	"row sk none cannot":   "39c9944d808839bd6f23982debd9fb43007834ef9d24c914e629d5baa1b58f76",
	"row sk stored can":    "4a34d876b786ec92ee266767bad2f1a634925bbfe0be48dc11510c3d5eda8f97",
	"row sk stored cannot": "faa5716934af3cabedfae4c1d80add450ca4c2b34718a2bd524f731a33438a1f",
	"row sl none can":      "2459d9c33d92c11013cb3a43fbcb7635cffdd5ef63238b194a8428f5931e425a",
	"row sl none cannot":   "097e7334183143bbf984fb07b6b27e31b26fc6754e5c5ce556d022e0d7c4a709",
	"row sl stored can":    "3a45b39eff5eced0e918475d6ab94c1351e3e1d38691d05877c8341d0c8a3899",
	"row sl stored cannot": "3472578697eb53714b7527d8a9f614626662cdc50747ee674d9f0239e702f46a",
	"row sn none can":      "26b117124943da53a74c2999d667cf5697d3411e130a0b2563611010cf5ed1f6",
	"row sn none cannot":   "2fe314ecdb4aff4c6e6f704ed49b62bd24782c85f50df1d521f151cf6ad7927f",
	"row sn stored can":    "1c7bc3764fd5ee761e54f12a9f8aed8019926191f3e1862647727d22f68a34c6",
	"row sn stored cannot": "0b6bf97b08c1ada082f0e172abe10a44fe8f6875a0bbffbcb0247b54f07b1da1",
	"row so none can":      "2a72fed563564952d608732097b4064914a9e44c1c458e3151dcd7b5e85d3f06",
	"row so none cannot":   "bf68a8b431c6a08f93010fc0fafc6173b99cb3a094a8854d8aea422b9a8349d6",
	"row so stored can":    "54c0f47abd20f0bc29b1337ae85d3e797cc639dab80d3f86297404e8c1208cc5",
	"row so stored cannot": "d1c4ca7537dd29f869f45f6bb9f8197ef76036de626c66ef986e00c1218acd59",
	"row sp none can":      "d42d1855e67e68b088b4927744f3a77f117defa9a14171327cf9d1c9866e2d6a",
	"row sp none cannot":   "e941b49c83e6c0fed790b6802980ef0b8f69f456e69ed153c48721580bcf6396",
	"row sp stored can":    "93a0c072624dcb97aadbef46a7cb6afd684c98515f0a2e890c5d82a9ef9494fb",
	"row sp stored cannot": "c924b16918f9bd5e94faa338e40b377641e48560956fce9fe224dd4bdcde4aab",
	"row sv none can":      "bff0b244834bfabaab62cce1198c58cf5ba60c9aac343843cf99630ee45cad9b",
	"row sv none cannot":   "77aaf38e98c29627e42f41ade4cedb4e71882725ade71159e5882d3a5f05667a",
	"row sv stored can":    "93d2ea58b80cb111e39252583df6a1d2668cfc59804fd01649e5b8bd711639ce",
	"row sv stored cannot": "b6548986229b812b8740de59676120fb9742eac9af1feea816e2dfaf30b45116",
}

// settingsDelGoldenData is the data of a state of the group: no rule at all, or every Phase 16 row set
// to its first preset (and the shared /cleanservice flag on, so joined and left read it too).
func settingsDelGoldenData(stored bool, rows []deletionRow) settingsDelData {
	data := settingsDelData{Rules: &deletion.Rules{ChatID: 1, Delays: map[string]int{}}}
	if !stored {
		return data
	}
	for _, row := range rows {
		data.Rules.Delays[row.Kind] = row.Presets[0]
	}
	data.Flag = true
	return data
}

// settingsDelGoldenDigest is the digest of one screen: its text and its keyboard as JSON.
func settingsDelGoldenDigest(t *testing.T, screen settingsScreen) string {
	t.Helper()
	keyboard, err := json.Marshal(screen.Keyboard)
	if err != nil {
		t.Fatalf("marshal keyboard: %v", err)
	}
	sum := sha256.Sum256([]byte(screen.Text + "\n--\n" + string(keyboard)))
	return hex.EncodeToString(sum[:])
}

// settingsDelGoldenScreens renders the two Phase 16 lists and the 17 Phase 16 row screens, each with no
// rule and with its first preset stored, each with no right line and with the cannot-delete line.
func settingsDelGoldenScreens(t *testing.T) map[string]settingsScreen {
	t.Helper()
	tr := i18n.MustNewTranslator("en")
	rights := map[string]string{"can": "", "cannot": "🚫 " + settingsText(tr, "settings_del_no_right")}
	sections := map[string][]deletionRow{"s": deletionServiceRows, "b": deletionBotRows}
	got := map[string]settingsScreen{}
	for rightName, right := range rights {
		for _, stored := range []bool{false, true} {
			state := map[bool]string{false: "none", true: "stored"}[stored]
			for code, rows := range sections {
				section, ok := settingsDelSectionByCode(code)
				if !ok {
					t.Fatalf("no section %s", code)
				}
				data := settingsDelGoldenData(stored, rows)
				prep := settingsDelPrepared{View: settingsDelPureView(tr, true, "Fuku Test Group"), Data: data, Right: right}
				got[fmt.Sprintf("list %s %s %s", code, state, rightName)] = renderSettingsDelList(prep, section, "")
				for _, row := range rows {
					got[fmt.Sprintf("row %s %s %s", row.Code, state, rightName)] = renderSettingsDelRow(prep, section, row, "")
				}
			}
		}
	}
	return got
}

// TestSettingsDeletionPhase16ScreensUnchanged is the characterization of the Phase 16 screens: every
// list and row screen renders exactly as it did before the Member messages section existed.
func TestSettingsDeletionPhase16ScreensUnchanged(t *testing.T) {
	withRealLocales(t)
	got := settingsDelGoldenScreens(t)
	if len(got) != 2*2*(2+len(deletionServiceRows)+len(deletionBotRows)) {
		t.Fatalf("rendered %d screens, want %d", len(got), 2*2*(2+len(deletionServiceRows)+len(deletionBotRows)))
	}
	if len(settingsDelGoldenDigests) == 0 {
		names := make([]string, 0, len(got))
		for name := range got {
			names = append(names, name)
		}
		sort.Strings(names)
		var lines []string
		for _, name := range names {
			lines = append(lines, fmt.Sprintf("\t%q: %q,", name, settingsDelGoldenDigest(t, got[name])))
		}
		t.Fatalf("no recorded digests; the screens are:\n%s", strings.Join(lines, "\n"))
	}
	for name, screen := range got {
		want, ok := settingsDelGoldenDigests[name]
		if !ok {
			t.Errorf("%s: no recorded digest", name)
			continue
		}
		if digest := settingsDelGoldenDigest(t, screen); digest != want {
			keyboard, _ := json.Marshal(screen.Keyboard)
			t.Errorf("%s: the screen changed (digest %s, recorded %s)\n%s\n%s", name, digest, want, screen.Text, keyboard)
		}
	}
	for name := range settingsDelGoldenDigests {
		if _, ok := got[name]; !ok {
			t.Errorf("%s: recorded but not rendered", name)
		}
	}
}
