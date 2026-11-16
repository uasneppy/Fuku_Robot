import { defineConfig } from "blume";

// Blume configuration for the Xenobot-72 documentation site.
// See https://useblume.dev/docs/configuration for the full reference.
export default defineConfig({
	title: "Xenobot-72",
	description:
		"A modern Telegram group management bot built with Go — 39 modules, 231 command names, 8 languages, and blazing-fast performance powered by Redis caching and PostgreSQL.",
	logo: { image: "/favicon.svg", text: "Xenobot-72" },
	github: { owner: "uasneppy", repo: "fuku_robot" },

	// Content stays where Starlight had it; the Go docs generator still
	// writes here (output path unchanged).
	content: { root: "src/content/docs" },

	// Static output keeps the existing Cloudflare Workers deployment as-is.
	deployment: {
		output: "static",
		site: "https://uasneppy.github.io/fuku_robot",
	},

	// Built-in AI artifacts (on by default): llms.txt, llms-full.txt, and a
	// raw .md/.mdx mirror of every page. No API key needed for these.
	ai: { llmsTxt: true },

	seo: {
		og: { enabled: true },
		sitemap: true,
		robots: true,
		structuredData: true,
	},

	// Collapsible sidebar groups, matching the previous Starlight layout.
	navigation: { sidebar: { display: "group" } },
});
