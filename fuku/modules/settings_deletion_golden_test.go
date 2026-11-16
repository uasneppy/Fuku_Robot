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
	"list b none cannot":   "c2502d33bf591c3bef0a388170385496829951d3809fe0222fcda0ff10cfed04",
	"list b stored can":    "99fdc9644f6cf7dbc63538270d14335e2ef974103f00dc8d112867fbbe50f4c3",
	"list b stored cannot": "fa62fa55083c9bf497adf58905c0a6525d5a3a5bfce785d2bb324f173cf99d06",
	"list s none can":      "5a5f0229ea326971cc6eb10a8a8ca486e7875af237a9b16328c0884151cc69ad",
	"list s none cannot":   "a7aecedc6d4d63a78a8e1492eea1f24664b837d5f8bbeeec40add0d9aee63afc",
	"list s stored can":    "d8d84b33dbc67a60d39cf66f71d00668d683423345358c43aa7f04a7ca18c8fa",
	"list s stored cannot": "cf6fb362adf364c8f7e0ffa1115a64115d22421bb035efc9d1090a7d0967ee56",
	"row ba none can":      "3e368a19fe214e39188616d0b3ae536605db4291a5642758db28d9de05fa9acf",
	"row ba none cannot":   "b2b3f7e1e390e02ca4ebc8c87580fa08a799d6c0719c1e167f5cc854ff4288b4",
	"row ba stored can":    "90e49ea1ab54a8bbca04c8fefc52790da5beab5b44677f3c6a9fdd3041221b4d",
	"row ba stored cannot": "2090c5b9a064c3f1cf0bef6be46d859aa883d85098380681c9ba12828b7e0407",
	"row bg none can":      "b6db3c1dcb59546f97835dd7ba04a4fb8b4243a6a784ace6bafa974cdf03f207",
	"row bg none cannot":   "dd0320e6f0f3d9e4811a6caec3b97f9ec1328393f5f1aaed97045c840f27e6c7",
	"row bg stored can":    "e82a2e05cc2a2f0cf9cba821e5d9d36eb1eb3d73b885b03ddce98db6030ee4f2",
	"row bg stored cannot": "ec078e0e32c04abf78a90d8481633e4ad1c00e79e036b2bb1774157bb8d3825e",
	"row bm none can":      "49ea8db1a85274e29ba15b53dbcbe4f471dcb04c2e7fa4a89631f8de0f52f4a6",
	"row bm none cannot":   "3722f45e013923a630ca4d520108a9cef9809f26d4f4ae2b8ef0306ee253e7b2",
	"row bm stored can":    "9e7eaf547df10695e71aee15cadf0e275b958dc031cb6817fb19553949d6249c",
	"row bm stored cannot": "c9cb2a29cc8fd8ada65a1caf56cd09545368ccef78534ea395f9c242c2d0d95e",
	"row br none can":      "5a3a45ef5afc7eed8c772d6aa3b22d4c04919cfe00919403c3293ee11d293f3f",
	"row br none cannot":   "6ab52040a0463fbccd66be65b7281156a68f3c50698485856cb3cd04dbfdebf1",
	"row br stored can":    "d2039d3a8b0022504869030cb7cb015c3bd54d5aa883d2f68b7da78a032a207c",
	"row br stored cannot": "f53e8fcbaee0ea77d7e94fcc925616e3112a29151727a778698156d0a4595c1f",
	"row bw none can":      "750d704f013cadf7ddac7da4c2e5af8c8617e585898b0b5fe5f7aca18b3cdffc",
	"row bw none cannot":   "ffd241effba7172353688f712385d2a4237e54f0d15b72b43588b551525ffc60",
	"row bw stored can":    "0294d41b778682982a9bd31bff18d9e865808e02da3e07bed853697728423d9e",
	"row bw stored cannot": "0da6ae97932e9fd33953043385e0438c4ba9d6ac476fbc554b9fd0fee06198be",
	"row sa none can":      "8bb9bfadec76c256002857e2c7ebba991598218ae9462d18b6902aa21c124278",
	"row sa none cannot":   "ed51e7c619b63bc2ff5ac138f817ae0b82ea2e58a1f3c1599fd392d44aa84b4c",
	"row sa stored can":    "6950e7f2f4828844adb6ac3f8e3465a8842653e86d3a46806b710b23d9020f91",
	"row sa stored cannot": "294a34b121f024617b06bd05e7ed838dbb81b0fece0b34cabff47bc32dd1d6a7",
	"row sb none can":      "1bb12ad998699ccec422f6ec2b04eecaa4c9611ef88ff4fa1768bc4f507cdf60",
	"row sb none cannot":   "bad094b3f8ffb74df66ac94e1f035d7865d2e2578666e13d69b4736525996ede",
	"row sb stored can":    "0baca68b9fd7207c4aefd84b8d70d9f95869a6c26cf6628ac64235ce7a24fdf4",
	"row sb stored cannot": "ab021e6488af39d024959b70a7f3f39ca22337e8ecec2d0080b4298193157a7c",
	"row sf none can":      "cbd533606c5cfd5ea037f9d8e5425f99bb7d16675fd2c7247a8fadca36082c4c",
	"row sf none cannot":   "7b3ba193301fb19b32d852d97903915cf3f3373be4607fe4d5d3607dbbde4983",
	"row sf stored can":    "016654d75c594c9f0802aec70260db5e7ee45457e316c5c88b9a450b792aadd2",
	"row sf stored cannot": "9f2bc4990cf9bb191fd30fb6b2907d10918d88c9b957196bb5b14e3cbcda0c5e",
	"row sg none can":      "231191304d463a16fb6166c775d3e755feac992775a2be9fc2d58923eeb2a400",
	"row sg none cannot":   "f2e550a6a40b1b2f49254c1f4fe1b5795c246a7e71c8da23dcb8e04c8228b14d",
	"row sg stored can":    "68fdde742440fbf4cb89d35cffaae2d2325ba6a33722f28350892f1c693c4731",
	"row sg stored cannot": "ee88a883b46219338ab69945a303ba28ffb0245142afb729dbbd3ebd95016a0f",
	"row sh none can":      "25122b60a91f6e8288bae100ceaf80f28cb718f38468c92bee77c44d5246084c",
	"row sh none cannot":   "36fd79a7d9f239b0268bf8905b8d31841a0a4ba736511bdd7ebfa32cb12c5b3d",
	"row sh stored can":    "5a7f3ab54efce1a79eed026dcbf662bd4c7a5e9d88c3c019177783fea22ce9f2",
	"row sh stored cannot": "7e9de0b7a2356c753c3b73487cdf23e2eb979d54e1e725be68ad6d2ac25da917",
	"row sj none can":      "7aba03ed766a585bf14dbed353bab379c972cf9240f667a4f27e7a7b6b7ee5ac",
	"row sj none cannot":   "ec27e57d81e58f1c9f3c8cddc43eb8a9fcdd07bbed8353b1e286e960282d7ae0",
	"row sj stored can":    "8418aefa629d282ee4383679e61ca9918b32f1a929a4bc8226794acc8c4f962c",
	"row sj stored cannot": "9eaedd8fe3ef206c5cf00b19af850752499ad9d6ec098cc174667964c440b87d",
	"row sk none can":      "e2e3ffc3186681c6c4c6517dac1f53eccb807d29ef9c7505c2d09f58e8d18d01",
	"row sk none cannot":   "135e1ff71f29327c8d987c19dab0da9199df0674704294c37237aa4e3965b399",
	"row sk stored can":    "ae4cbe2b237853b65b0fa227c04efa65dac75b797b4c1320da810fe009b8b92d",
	"row sk stored cannot": "4884f872fe838ed4e1ef880ed7c33bad70abaed148bfaaa5a41eeae8b4194a2d",
	"row sl none can":      "2459d9c33d92c11013cb3a43fbcb7635cffdd5ef63238b194a8428f5931e425a",
	"row sl none cannot":   "a623198a5bc7c952fcbf97a9d912364511e9c47e650daab46c04f69f5d2f693b",
	"row sl stored can":    "3a45b39eff5eced0e918475d6ab94c1351e3e1d38691d05877c8341d0c8a3899",
	"row sl stored cannot": "81491daef64a177ce2f03d41d14c6863460b8f4ef15643ad0d52cb6b1d277b00",
	"row sn none can":      "26b117124943da53a74c2999d667cf5697d3411e130a0b2563611010cf5ed1f6",
	"row sn none cannot":   "8fb9b373b79fcb2d34eb336f98122a4051e017348f425449af3e00f8386017f8",
	"row sn stored can":    "1c7bc3764fd5ee761e54f12a9f8aed8019926191f3e1862647727d22f68a34c6",
	"row sn stored cannot": "c593326454634acf6cc39838f9073f3ad1592f277d7b8c6e0c5b33042fb203fa",
	"row so none can":      "2a72fed563564952d608732097b4064914a9e44c1c458e3151dcd7b5e85d3f06",
	"row so none cannot":   "a6c449c889f62c2f61bcab1626126f8c8fb597ccf22fb7260afa0adad5b049cd",
	"row so stored can":    "54c0f47abd20f0bc29b1337ae85d3e797cc639dab80d3f86297404e8c1208cc5",
	"row so stored cannot": "7a9d2116f7f283623e773e9233bfeb4ec07b904e745c945740a29c5bff39a5a8",
	"row sp none can":      "accf3f7f949f2928847c8fda8b513fce3133edb19d8c392ee48a0359aee11d3e",
	"row sp none cannot":   "44718a8353ac9f671a84f6262c86ba65b8bcecce5e5721f1d5d105cefd949608",
	"row sp stored can":    "d573db6ab5d1f0dfc9b66a3888348e9f32505c9bcc52955823169fab0b0bfedd",
	"row sp stored cannot": "adff2d87f1fbcd456a04667dd114ecde2cf4ae58ee5371884dec59e743258ba3",
	"row sv none can":      "bff0b244834bfabaab62cce1198c58cf5ba60c9aac343843cf99630ee45cad9b",
	"row sv none cannot":   "d9e331391b5997106a44300fb2d1cc0d9875c3501f6218079d70faf8bd3fb462",
	"row sv stored can":    "93d2ea58b80cb111e39252583df6a1d2668cfc59804fd01649e5b8bd711639ce",
	"row sv stored cannot": "1cb705dd906a577d1ede3d35d36ff925f601050d99dc61a3945048695fe8518f",
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
