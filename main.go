package main

import (
	"log"
	"time"
	"encoding/json"
	"net/http"
	"sort"
	"os"
	"strings"
	//"strconv"
  "github.com/bwmarrin/discordgo"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/joho/godotenv"
)
var recentCommits []GHApiCommit
var discordToken string
var discordStatusChannel string
var discordUpdateChannel string
var currentStatus Status
var currentUpdate Update

type Update struct {
	ID           string `json:"id"`
	GuildID      string `json:"guildId"`
	ChannelID    string `json:"channelId"`
	CreatedTs    int64  `json:"createdTimestamp"`
	EditedTs     int64  `json:"editedTimestamp"`
	AuthorID     string `json:"authorId"`
	AuthorName   string `json:"authorName"`
	AuthorImage  string `json:"authorImage"`
	Content      string `json:"content"`
	CleanContent string `json:"cleanContent"`
	Image        string `json:"image"`
}
type Status struct {
	Type string `json:"type"`
	Text string `json:"text"`
}


type GHAuthor struct {
	Login     string `json:"login"`
	AvatarUrl string `json:"avatar_url"`
	HtmlUrl   string `json:"html_url"`
}

type GHCommit struct {
	Author  GHCommitAuthor `json:"author"`
	Message string         `json:"message"`
}

type GHCommitAuthor struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Date  string `json:"date"`
}

type GHApiCommit struct {
	Author  GHAuthor `json:"author"`
	Commit  GHCommit `json:"commit"`
	HtmlUrl string   `json:"html_url"`
}

type GHCommitsByDate []GHApiCommit

func (d GHCommitsByDate) Len() int      { return len(d) }
func (d GHCommitsByDate) Swap(i, j int) { d[i], d[j] = d[j], d[i] }
func (d GHCommitsByDate) Less(i, j int) bool {
	time1, _ := time.Parse(time.RFC3339, d[i].Commit.Author.Date)
	time2, _ := time.Parse(time.RFC3339, d[j].Commit.Author.Date)
	return time1.After(time2)
}


func backgroundTask() {
	for {
		time.Sleep(15 * time.Minute)
		//getRecentsCommits()
	}
}

func startDiscordBot() {
	// create Discord session
	dg, err := discordgo.New("Bot " + discordToken)
	if err != nil {
		log.Fatalf("failed to create Discord session: %s", err)
	}

	// set intents (all we want is guild messages)
	dg.Identify.Intents = discordgo.IntentsGuildMessages

	// get current status
	statusMessages, err := dg.ChannelMessages(discordStatusChannel, 0, "", "", "")
	if err != nil {
		log.Fatalf("failed getting current status: %s", err)
	}
	for i := 0; i < len(statusMessages); i++ {
		m := statusMessages[i]
		if strings.HasPrefix(m.Content, "--status-set") {
			currentStatus = Status{
				Type: "warn",
				Text: strings.TrimSpace(strings.Replace(m.Content, "--status-set", "", 1)),
			}
			break
		} else if strings.HasPrefix(m.Content, "--status-remove") {
			currentStatus = Status{
				Type: "empty",
			}
			break
		}
	}

	// get latest update
	updateMessages, err := dg.ChannelMessages(discordUpdateChannel, 0, "", "", "")
	if err != nil {
		log.Fatalf("failed getting latest update: %s", err)
	}
	for i := 0; i < len(updateMessages); i++ {
		m := updateMessages[i]
		if len(m.Attachments) > 0 {
			currentUpdate = Update{
				ID:           m.ID,
				GuildID:      m.GuildID,
				ChannelID:    m.ChannelID,
				CreatedTs:    m.Timestamp.UnixMilli(),
				EditedTs:     m.Timestamp.UnixMilli(),
				AuthorID:     m.Author.ID,
				AuthorName:   m.Author.Username,
				AuthorImage:  m.Author.AvatarURL(""),
				Content:      m.Content,
				CleanContent: m.ContentWithMentionsReplaced(),
				Image:        m.Attachments[0].URL,
			}
			break
		}
	}

	// messageCreate handler
	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.ChannelID == discordStatusChannel {
			log.Printf("updating status by request of %s", m.Author.Username)
			if strings.HasPrefix(m.Content, "--status-set") {
				currentStatus = Status{
					Type: "warn",
					Text: strings.TrimSpace(strings.Replace(m.Content, "--status-set", "", 1)),
				}
				s.MessageReactionAdd(m.ChannelID, m.ID, "<:good:1118293837773807657>")
			} else if strings.HasPrefix(m.Content, "--status-remove") {
				currentStatus = Status{
					Type: "empty",
				}
				s.MessageReactionAdd(m.ChannelID, m.ID, "<:good:1118293837773807657>")
			}
		} else if m.ChannelID == discordUpdateChannel && len(m.Attachments) > 0 {
			log.Printf("updating latest update by request of %s", m.Author.Username)
			currentUpdate = Update{
				ID:           m.ID,
				GuildID:      m.GuildID,
				ChannelID:    m.ChannelID,
				CreatedTs:    m.Timestamp.UnixMilli(),
				EditedTs:     m.Timestamp.UnixMilli(),
				AuthorID:     m.Author.ID,
				AuthorName:   m.Author.Username,
				AuthorImage:  m.Author.AvatarURL(""),
				Content:      m.Content,
				CleanContent: m.ContentWithMentionsReplaced(),
				Image:        m.Attachments[0].URL,
			}
		}
	})

	// connect to Discord
	err = dg.Open()
	if err != nil {
		log.Fatalf("failed to open Discord connection: %s", err)
	}
	defer dg.Close()
}


func main() {
	// Load dotenv
	if err := godotenv.Load(); err != nil {
		log.Fatal("No .env file found!")
	}
	discordToken = os.Getenv("DISCORD_TOKEN")
	discordStatusChannel = os.Getenv("DISCORD_STATUS_CHANNEL")
	discordUpdateChannel = os.Getenv("DISCORD_UPDATES_CHANNEL")

	// get recent commits
	getRecentsCommits()

	// start Discord bot
  go startDiscordBot()

	// start background task
	go backgroundTask()

	// initialize fiber app
	app := fiber.New()
	app.Use(cors.New())

	// root endpoint
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("online")
	})

	// status endpoint
	app.Get("/status", func(c *fiber.Ctx) error {
		return c.JSON(currentStatus)
	})

	// latest updates endpoint
	app.Get("/updates", func(c *fiber.Ctx) error {
		return c.JSON(currentUpdate)
	})

	// recent commits endpoint
	app.Get("/commits", func(c *fiber.Ctx) error {
		return c.JSON(recentCommits)
	})

	// start fiber app
	app.Listen(":3000")
}

func getRecentsCommits() {
	githubCommitApis := []string{
		"https://api.github.com/repos/ScratchTurbo/ScratchTurbo-Home/commits?per_page=50",
		"https://api.github.com/repos/ScratchTurbo/ScratchTurbo-API/commits?per_page=50",
		"https://api.github.com/repos/ScratchTurbo/ScratchTurbo-BasicAPI/commits?per_page=50",
		"https://api.github.com/repos/ScratchTurbo/ScratchTurbo-Docs/commits?per_page=50",
		"https://api.github.com/repos/ScratchTurbo/ScratchTurbo-ObjectLibraries/commits?per_page=50",
		"https://api.github.com/repos/ScratchTurbo/ScratchTurbo-Packager/commits?per_page=50",
		"https://api.github.com/repos/ScratchTurbo/ScratchTurbo-Studios/commits?per_page=50",
	}

	var newRecentCommits []GHApiCommit
	for i := 0; i < len(githubCommitApis); i++ {
		resp, err := http.Get(githubCommitApis[i])
		if err != nil {
			//log.Errorf("Failed fetching %s: %s", githubCommitApis[i], err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			//log.Errorf("Failed fetching %s: Non-OK status code; %s", githubCommitApis[i], strconv.Itoa(resp.StatusCode))
			continue
		}

		var apiResp []GHApiCommit
		err = json.NewDecoder(resp.Body).Decode(&apiResp)
		if err != nil {
			//log.Errorf("Failed decoding response from %s: %s", githubCommitApis[i], err)
			continue
		}

		newRecentCommits = append(newRecentCommits, apiResp...)
	}

	sort.Sort(GHCommitsByDate(newRecentCommits))
	if len(newRecentCommits) >= 200 {
		recentCommits = newRecentCommits[:200]
	} else {
		recentCommits = newRecentCommits
	}
}

