package mock

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
)

type Handler func(req *http.Request) (body []byte, statusCode int)

type MockSession struct {
	Session  *discordgo.Session
	t        TestReporter
	mu       sync.Mutex
	handlers map[string]Handler
}

type TestReporter interface {
	Helper()
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
}

// NewSession creates a MockSession with a pre-configured discordgo.Session.
// The session's State is initialized from NewState() and must be populated
// by the caller using the Bot helper methods (GuildAdd, MemberAdd, etc.)
// or by calling WithGuild/WithMember.
//
// The session's HTTP client is wired to an httptest.Server whose behavior
// is controlled by the Handlers map.
func NewSession(t TestReporter) *MockSession {
	t.Helper()

	s, err := discordgo.New("Bot testtoken")
	if err != nil {
		t.Fatalf("creating mock session: %v", err)
	}

	s.StateEnabled = true
	s.State = discordgo.NewState()
	s.Ratelimiter = discordgo.NewRatelimiter()

	ms := &MockSession{
		Session:  s,
		t:        t,
		handlers: make(map[string]Handler),
	}

	srv := httptest.NewServer(http.HandlerFunc(ms.serve))
	s.Client = srv.Client()

	// Override the discordgo endpoint base URLs so that all API requests
	// resolve to our test server instead of https://discord.com.
	// The endpoints are package-level vars and thus reassignable.
	srvURL := srv.URL
	discordgo.EndpointDiscord = srvURL + "/"
	discordgo.EndpointAPI = srvURL + "/api/v" + discordgo.APIVersion + "/"
	discordgo.EndpointGuilds = srvURL + "/api/v" + discordgo.APIVersion + "/guilds/"
	discordgo.EndpointUsers = srvURL + "/api/v" + discordgo.APIVersion + "/users/"
	discordgo.EndpointChannels = srvURL + "/api/v" + discordgo.APIVersion + "/channels/"
	discordgo.EndpointCDN = srvURL + "/cdn/"

	return ms
}

func (ms *MockSession) serve(w http.ResponseWriter, r *http.Request) {
	ms.mu.Lock()
	handler := ms.match(normalizePath(r.URL.Path))
	ms.mu.Unlock()

	if handler == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]any{
			"message": "unknown endpoint",
			"code":    0,
		})
		return
	}

	body, status := handler(r)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if len(body) > 0 {
		w.Write(body)
	}
}

func normalizePath(p string) string {
	// Match patterns like /api/v9, /api/v10, etc.
	parts := strings.SplitN(p, "/", 4)
	if len(parts) >= 4 && parts[1] == "api" && strings.HasPrefix(parts[2], "v") {
		return "/" + parts[3]
	}
	return p
}

func (ms *MockSession) match(path string) Handler {
	if h, ok := ms.handlers[path]; ok {
		return h
	}

	bestKey := ""
	for key := range ms.handlers {
		if strings.HasPrefix(path, key) {
			if len(key) > len(bestKey) {
				bestKey = key
			}
		}
	}
	if bestKey != "" {
		return ms.handlers[bestKey]
	}
	return nil
}

func (ms *MockSession) On(pathPrefix string, handler Handler) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.handlers[pathPrefix] = handler
}

func (ms *MockSession) Close() {
	ms.t.Helper()
	ms.Session.Client = http.DefaultClient
}

func (ms *MockSession) WithBotUser(userID, username string) *MockSession {
	ms.Session.State.User = &discordgo.User{
		ID:       userID,
		Username: username,
		Bot:      true,
	}
	return ms
}

func (ms *MockSession) WithGuild(guild *discordgo.Guild) *MockSession {
	if err := ms.Session.State.GuildAdd(guild); err != nil {
		ms.t.Fatalf("adding guild to state: %v", err)
	}
	return ms
}

func (ms *MockSession) WithMember(guildID string, member *discordgo.Member) *MockSession {
	if err := ms.Session.State.MemberAdd(member); err != nil {
		ms.t.Fatalf("adding member to state: %v", err)
	}
	return ms
}

func (ms *MockSession) WithChannel(ch *discordgo.Channel) *MockSession {
	if err := ms.Session.State.ChannelAdd(ch); err != nil {
		ms.t.Fatalf("adding channel to state: %v", err)
	}
	return ms
}

func JSON(val any, status int) Handler {
	return func(req *http.Request) ([]byte, int) {
		b, _ := json.Marshal(val)
		return b, status
	}
}

// APIPath extracts the Discord API path portion from a full URL.
// For example, "https://discord.com/api/v9/channels/123" -> "/channels/123"
func APIPath(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	// Strip "/api/v9" prefix to get the normalized path.
	return u.Path
}
