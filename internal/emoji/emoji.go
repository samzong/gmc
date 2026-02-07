package emoji

import (
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Gitmoji represents a single gitmoji entry from gitmoji.dev specification
type Gitmoji struct {
	Emoji       string
	Code        string
	Description string
	Name        string
	Semver      string // "major", "minor", "patch", or ""
}

// gitmojis contains the complete gitmoji.dev specification
// Source: https://github.com/carloscuesta/gitmoji
//
//nolint:lll // data table from upstream spec, line length is unavoidable
var gitmojis = []Gitmoji{
	{Emoji: "🎨", Code: ":art:", Description: "Improve structure / format of the code", Name: "art", Semver: ""},
	{Emoji: "⚡️", Code: ":zap:", Description: "Improve performance", Name: "zap", Semver: "patch"},
	{Emoji: "🔥", Code: ":fire:", Description: "Remove code or files", Name: "fire", Semver: ""},
	{Emoji: "🐛", Code: ":bug:", Description: "Fix a bug", Name: "bug", Semver: "patch"},
	{Emoji: "🚑️", Code: ":ambulance:", Description: "Critical hotfix", Name: "ambulance", Semver: "patch"},
	{Emoji: "✨", Code: ":sparkles:", Description: "Introduce new features", Name: "sparkles", Semver: "minor"},
	{Emoji: "📝", Code: ":memo:", Description: "Add or update documentation", Name: "memo", Semver: ""},
	{Emoji: "🚀", Code: ":rocket:", Description: "Deploy stuff", Name: "rocket", Semver: ""},
	{Emoji: "💄", Code: ":lipstick:", Description: "Add or update the UI and style files", Name: "lipstick", Semver: "patch"},
	{Emoji: "🎉", Code: ":tada:", Description: "Begin a project", Name: "tada", Semver: ""},
	{Emoji: "✅", Code: ":white_check_mark:", Description: "Add, update, or pass tests", Name: "white-check-mark", Semver: ""},
	{Emoji: "🔒️", Code: ":lock:", Description: "Fix security or privacy issues", Name: "lock", Semver: "patch"},
	{Emoji: "🔐", Code: ":closed_lock_with_key:", Description: "Add or update secrets", Name: "closed-lock-with-key", Semver: ""},
	{Emoji: "🔖", Code: ":bookmark:", Description: "Release / Version tags", Name: "bookmark", Semver: ""},
	{Emoji: "🚨", Code: ":rotating_light:", Description: "Fix compiler / linter warnings", Name: "rotating-light", Semver: ""},
	{Emoji: "🚧", Code: ":construction:", Description: "Work in progress", Name: "construction", Semver: ""},
	{Emoji: "💚", Code: ":green_heart:", Description: "Fix CI Build", Name: "green-heart", Semver: ""},
	{Emoji: "⬇️", Code: ":arrow_down:", Description: "Downgrade dependencies", Name: "arrow-down", Semver: "patch"},
	{Emoji: "⬆️", Code: ":arrow_up:", Description: "Upgrade dependencies", Name: "arrow-up", Semver: "patch"},
	{Emoji: "📌", Code: ":pushpin:", Description: "Pin dependencies to specific versions", Name: "pushpin", Semver: "patch"},
	{Emoji: "👷", Code: ":construction_worker:", Description: "Add or update CI build system", Name: "construction-worker", Semver: ""},
	{Emoji: "📈", Code: ":chart_with_upwards_trend:", Description: "Add or update analytics or track code", Name: "chart-with-upwards-trend", Semver: "patch"},
	{Emoji: "♻️", Code: ":recycle:", Description: "Refactor code", Name: "recycle", Semver: ""},
	{Emoji: "➕", Code: ":heavy_plus_sign:", Description: "Add a dependency", Name: "heavy-plus-sign", Semver: "patch"},
	{Emoji: "➖", Code: ":heavy_minus_sign:", Description: "Remove a dependency", Name: "heavy-minus-sign", Semver: "patch"},
	{Emoji: "🔧", Code: ":wrench:", Description: "Add or update configuration files", Name: "wrench", Semver: "patch"},
	{Emoji: "🔨", Code: ":hammer:", Description: "Add or update development scripts", Name: "hammer", Semver: ""},
	{Emoji: "🌐", Code: ":globe_with_meridians:", Description: "Internationalization and localization", Name: "globe-with-meridians", Semver: "patch"},
	{Emoji: "✏️", Code: ":pencil2:", Description: "Fix typos", Name: "pencil2", Semver: "patch"},
	{Emoji: "💩", Code: ":poop:", Description: "Write bad code that needs to be improved", Name: "poop", Semver: ""},
	{Emoji: "⏪️", Code: ":rewind:", Description: "Revert changes", Name: "rewind", Semver: "patch"},
	{Emoji: "🔀", Code: ":twisted_rightwards_arrows:", Description: "Merge branches", Name: "twisted-rightwards-arrows", Semver: ""},
	{Emoji: "📦️", Code: ":package:", Description: "Add or update compiled files or packages", Name: "package", Semver: "patch"},
	{Emoji: "👽️", Code: ":alien:", Description: "Update code due to external API changes", Name: "alien", Semver: "patch"},
	{Emoji: "🚚", Code: ":truck:", Description: "Move or rename resources (e.g.: files, paths, routes)", Name: "truck", Semver: ""},
	{Emoji: "📄", Code: ":page_facing_up:", Description: "Add or update license", Name: "page-facing-up", Semver: ""},
	{Emoji: "💥", Code: ":boom:", Description: "Introduce breaking changes", Name: "boom", Semver: "major"},
	{Emoji: "🍱", Code: ":bento:", Description: "Add or update assets", Name: "bento", Semver: "patch"},
	{Emoji: "♿️", Code: ":wheelchair:", Description: "Improve accessibility", Name: "wheelchair", Semver: "patch"},
	{Emoji: "💡", Code: ":bulb:", Description: "Add or update comments in source code", Name: "bulb", Semver: ""},
	{Emoji: "🍻", Code: ":beers:", Description: "Write code drunkenly", Name: "beers", Semver: ""},
	{Emoji: "💬", Code: ":speech_balloon:", Description: "Add or update text and literals", Name: "speech-balloon", Semver: "patch"},
	{Emoji: "🗃️", Code: ":card_file_box:", Description: "Perform database related changes", Name: "card-file-box", Semver: "patch"},
	{Emoji: "🔊", Code: ":loud_sound:", Description: "Add or update logs", Name: "loud-sound", Semver: ""},
	{Emoji: "🔇", Code: ":mute:", Description: "Remove logs", Name: "mute", Semver: ""},
	{Emoji: "👥", Code: ":busts_in_silhouette:", Description: "Add or update contributor(s)", Name: "busts-in-silhouette", Semver: ""},
	{Emoji: "🚸", Code: ":children_crossing:", Description: "Improve user experience / usability", Name: "children-crossing", Semver: "patch"},
	{Emoji: "🏗️", Code: ":building_construction:", Description: "Make architectural changes", Name: "building-construction", Semver: ""},
	{Emoji: "📱", Code: ":iphone:", Description: "Work on responsive design", Name: "iphone", Semver: "patch"},
	{Emoji: "🤡", Code: ":clown_face:", Description: "Mock things", Name: "clown-face", Semver: ""},
	{Emoji: "🥚", Code: ":egg:", Description: "Add or update an easter egg", Name: "egg", Semver: "patch"},
	{Emoji: "🙈", Code: ":see_no_evil:", Description: "Add or update a .gitignore file", Name: "see-no-evil", Semver: ""},
	{Emoji: "📸", Code: ":camera_flash:", Description: "Add or update snapshots", Name: "camera-flash", Semver: ""},
	{Emoji: "⚗️", Code: ":alembic:", Description: "Perform experiments", Name: "alembic", Semver: "patch"},
	{Emoji: "🔍️", Code: ":mag:", Description: "Improve SEO", Name: "mag", Semver: "patch"},
	{Emoji: "🏷️", Code: ":label:", Description: "Add or update types", Name: "label", Semver: "patch"},
	{Emoji: "🌱", Code: ":seedling:", Description: "Add or update seed files", Name: "seedling", Semver: ""},
	{Emoji: "🚩", Code: ":triangular_flag_on_post:", Description: "Add, update, or remove feature flags", Name: "triangular-flag-on-post", Semver: "patch"},
	{Emoji: "🥅", Code: ":goal_net:", Description: "Catch errors", Name: "goal-net", Semver: "patch"},
	{Emoji: "💫", Code: ":dizzy:", Description: "Add or update animations and transitions", Name: "dizzy", Semver: "patch"},
	{Emoji: "🗑️", Code: ":wastebasket:", Description: "Deprecate code that needs to be cleaned up", Name: "wastebasket", Semver: "patch"},
	{Emoji: "🛂", Code: ":passport_control:", Description: "Work on code related to authorization, roles and permissions", Name: "passport-control", Semver: "patch"},
	{Emoji: "🩹", Code: ":adhesive_bandage:", Description: "Simple fix for a non-critical issue", Name: "adhesive-bandage", Semver: "patch"},
	{Emoji: "🧐", Code: ":monocle_face:", Description: "Data exploration/inspection", Name: "monocle-face", Semver: ""},
	{Emoji: "⚰️", Code: ":coffin:", Description: "Remove dead code", Name: "coffin", Semver: ""},
	{Emoji: "🧪", Code: ":test_tube:", Description: "Add a failing test", Name: "test-tube", Semver: ""},
	{Emoji: "👔", Code: ":necktie:", Description: "Add or update business logic", Name: "necktie", Semver: "patch"},
	{Emoji: "🩺", Code: ":stethoscope:", Description: "Add or update healthcheck", Name: "stethoscope", Semver: ""},
	{Emoji: "🧱", Code: ":bricks:", Description: "Infrastructure related changes", Name: "bricks", Semver: ""},
	{Emoji: "🧑‍💻", Code: ":technologist:", Description: "Improve developer experience", Name: "technologist", Semver: ""},
	{Emoji: "💸", Code: ":money_with_wings:", Description: "Add sponsorships or money related infrastructure", Name: "money-with-wings", Semver: ""},
	{Emoji: "🧵", Code: ":thread:", Description: "Add or update code related to multithreading or concurrency", Name: "thread", Semver: ""},
	{Emoji: "🦺", Code: ":safety_vest:", Description: "Add or update code related to validation", Name: "safety-vest", Semver: ""},
}

// conventionalToGitmoji maps conventional commit types to gitmoji names
var conventionalToGitmoji = map[string]string{
	"feat":      "sparkles",
	"fix":       "bug",
	"docs":      "memo",
	"style":     "lipstick",
	"refactor":  "recycle",
	"perf":      "zap",
	"test":      "white-check-mark",
	"build":     "construction-worker",
	"ci":        "green-heart",
	"chore":     "wrench",
	"revert":    "rewind",
	"deps":      "arrow-up",
	"security":  "lock",
	"hotfix":    "ambulance",
	"release":   "rocket",
	"wip":       "construction",
	"init":      "tada",
	"breaking":  "boom",
	"config":    "wrench",
	"i18n":      "globe-with-meridians",
	"typo":      "pencil2",
	"merge":     "twisted-rightwards-arrows",
	"move":      "truck",
	"remove":    "fire",
	"add":       "heavy-plus-sign",
	"upgrade":   "arrow-up",
	"downgrade": "arrow-down",
	"other":     "wrench",
}

var (
	gitmojiByName   map[string]*Gitmoji
	gitmojiByEmoji  map[string]*Gitmoji
	emojiPrefixes   []string
	commitTypeRegex *regexp.Regexp
	initOnce        sync.Once
)

func initMaps() {
	initOnce.Do(func() {
		gitmojiByName = make(map[string]*Gitmoji, len(gitmojis))
		gitmojiByEmoji = make(map[string]*Gitmoji, len(gitmojis))
		emojiPrefixes = make([]string, 0, len(gitmojis))

		for i := range gitmojis {
			g := &gitmojis[i]
			gitmojiByName[g.Name] = g
			gitmojiByEmoji[g.Emoji] = g
			emojiPrefixes = append(emojiPrefixes, g.Emoji)
		}

		sort.Slice(emojiPrefixes, func(i, j int) bool {
			return len(emojiPrefixes[i]) > len(emojiPrefixes[j])
		})

		commitTypeRegex = regexp.MustCompile(`^([a-zA-Z]+)(?:\([^)]+\))?:`)
	})
}

func GetAllGitmojis() []Gitmoji {
	return gitmojis
}

func GetGitmojiByName(name string) *Gitmoji {
	initMaps()
	return gitmojiByName[name]
}

func GetGitmojiByEmoji(emoji string) *Gitmoji {
	initMaps()
	return gitmojiByEmoji[emoji]
}

func GetEmojiForType(commitType string) string {
	initMaps()
	commitType = strings.ToLower(commitType)

	if name, ok := conventionalToGitmoji[commitType]; ok {
		if g := gitmojiByName[name]; g != nil {
			return g.Emoji
		}
	}
	return ""
}

func GetAllCommitTypes() []string {
	types := make([]string, 0, len(conventionalToGitmoji))
	for t := range conventionalToGitmoji {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}

func GetCommitTypesRegexPattern() string {
	types := GetAllCommitTypes()
	return strings.Join(types, "|")
}

func GetEmojiDescription() string {
	initMaps()
	types := GetAllCommitTypes()
	parts := make([]string, 0, len(types))
	for _, t := range types {
		if emoji := GetEmojiForType(t); emoji != "" {
			parts = append(parts, emoji+" for "+t)
		}
	}
	return strings.Join(parts, ", ")
}

func InferTypeFromEmojiPrefix(message string) (string, string) {
	initMaps()
	message = strings.TrimSpace(message)
	if message == "" {
		return "", ""
	}

	for _, emoji := range emojiPrefixes {
		if after, found := strings.CutPrefix(message, emoji); found {
			rest := strings.TrimSpace(after)
			if g := gitmojiByEmoji[emoji]; g != nil {
				for convType, gitmojiName := range conventionalToGitmoji {
					if gitmojiName == g.Name {
						return convType, rest
					}
				}
			}
			return "", rest
		}
	}
	return "", ""
}

func AddEmojiToMessage(message string) string {
	initMaps()
	message = strings.TrimSpace(message)
	if message == "" {
		return message
	}

	firstRune := []rune(message)[0]
	if isEmoji(firstRune) {
		return message
	}

	commitType := extractCommitType(message)
	if commitType == "" {
		return message
	}

	emoji := GetEmojiForType(commitType)
	if emoji == "" {
		return message
	}

	return emoji + " " + message
}

func extractCommitType(message string) string {
	initMaps()
	matches := commitTypeRegex.FindStringSubmatch(message)
	if len(matches) >= 2 {
		return strings.ToLower(matches[1])
	}
	return ""
}

func isEmoji(r rune) bool {
	return (r >= 0x1F000 && r <= 0x1F9FF) ||
		(r >= 0x2600 && r <= 0x26FF) ||
		(r >= 0x2700 && r <= 0x27BF) ||
		(r >= 0xFE00 && r <= 0xFE0F) ||
		(r == 0x200D) ||
		(r >= 0x203C && r <= 0x3299)
}

func GetGitmojiPromptList() string {
	var sb strings.Builder
	for _, g := range gitmojis[:20] {
		sb.WriteString(g.Emoji)
		sb.WriteString(" ")
		sb.WriteString(g.Description)
		sb.WriteString("\n")
	}
	return sb.String()
}
